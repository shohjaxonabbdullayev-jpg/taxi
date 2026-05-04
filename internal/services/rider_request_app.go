package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"taxi-mvp/internal/abuse"
	"taxi-mvp/internal/config"
	"taxi-mvp/internal/domain"
	"taxi-mvp/internal/legal"
	"taxi-mvp/internal/utils"
)

// Sentinel errors for native rider ride-request HTTP flow (Bearer auth).
// Handlers map these to JSON { error: { code, message } }.
var (
	ErrRiderRequestLegalRequired     = errors.New("rider_request: legal acceptance required")
	ErrRiderRequestPhoneRequired     = errors.New("rider_request: phone required")
	ErrRiderRequestAbuseBlocked      = errors.New("rider_request: abuse block active")
	ErrRiderRequestDuplicatePending  = errors.New("rider_request: already has pending request")
	ErrRiderRequestNotFound          = errors.New("rider_request: not found")
	ErrRiderRequestConflictState     = errors.New("rider_request: invalid state for operation")
	ErrRiderRequestInvalidCoords     = errors.New("rider_request: invalid coordinates")
	ErrRiderRequestMatchUnavailable  = errors.New("rider_request: dispatch service unavailable")
)

// RiderRequestAppService implements the same DB progression as the Telegram
// rider bot (internal/bot/rider/bot.go): pickup INSERT → destination UPDATE
// + server estimated_price → destination_confirmed + MatchService.BroadcastRequest.
type RiderRequestAppService struct {
	db      *sql.DB
	cfg     *config.Config
	match   *MatchService
}

// NewRiderRequestAppService wires the app ride-request use-case. match may be
// nil only in tests that never call Confirm; production should always pass a
// non-nil MatchService.
func NewRiderRequestAppService(db *sql.DB, cfg *config.Config, match *MatchService) *RiderRequestAppService {
	return &RiderRequestAppService{db: db, cfg: cfg, match: match}
}

func validPickupDropLatLng(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// CreatePickupRequest mirrors rider bot handleLocation INSERT (after phone +
// legal + abuse + single-PENDING checks). clientRequestID is accepted for
// forward-compatibility; it is not persisted yet.
func (s *RiderRequestAppService) CreatePickupRequest(ctx context.Context, riderUserID int64, pickupLat, pickupLng float64, clientRequestID string) (requestID string, err error) {
	_ = clientRequestID
	if s == nil || s.db == nil || s.cfg == nil {
		return "", ErrRiderRequestMatchUnavailable
	}
	if riderUserID <= 0 {
		return "", ErrRiderRequestNotFound
	}
	if !validPickupDropLatLng(pickupLat, pickupLng) {
		return "", ErrRiderRequestInvalidCoords
	}

	legalSvc := legal.NewService(s.db)
	if !legalSvc.RiderHasActiveLegal(ctx, riderUserID) {
		return "", ErrRiderRequestLegalRequired
	}

	var phone sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT phone FROM users WHERE id = ?1`, riderUserID).Scan(&phone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRiderRequestNotFound
		}
		return "", err
	}
	if !phone.Valid || strings.TrimSpace(phone.String) == "" {
		return "", ErrRiderRequestPhoneRequired
	}

	if penalty, err := abuse.CheckRiderBlock(ctx, s.db, riderUserID, time.Now()); err == nil && penalty != nil {
		return "", ErrRiderRequestAbuseBlocked
	}

	var existing int
	if err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM ride_requests WHERE rider_user_id = ?1 AND status = ?2 LIMIT 1`,
		riderUserID, domain.RequestStatusPending).Scan(&existing); err == nil {
		return "", ErrRiderRequestDuplicatePending
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	reqUUID := uuid.New()
	expiresAt := time.Now().Add(time.Duration(s.cfg.RequestExpiresSeconds) * time.Second)
	pickupGrid := utils.GridID(pickupLat, pickupLng)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, radius_km, status, expires_at, pickup_grid)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
		reqUUID.String(), riderUserID, pickupLat, pickupLng, s.cfg.MatchRadiusKm, domain.RequestStatusPending, expiresAt, pickupGrid)
	if err != nil {
		log.Printf("rider_request_app: create request: %v", err)
		return "", err
	}
	return reqUUID.String(), nil
}

// SetDestination mirrors the rider bot destination UPDATE (drop + server
// estimated_price + expiry refresh). Never trusts a client fare.
func (s *RiderRequestAppService) SetDestination(ctx context.Context, riderUserID int64, requestID string, dropLat, dropLng float64, dropName string) (estimatedPrice int64, err error) {
	if s == nil || s.db == nil || s.cfg == nil {
		return 0, ErrRiderRequestMatchUnavailable
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || riderUserID <= 0 {
		return 0, ErrRiderRequestNotFound
	}
	if !validPickupDropLatLng(dropLat, dropLng) {
		return 0, ErrRiderRequestInvalidCoords
	}

	var pickupLat, pickupLng float64
	var status string
	err = s.db.QueryRowContext(ctx, `
		SELECT pickup_lat, pickup_lng, status
		FROM ride_requests WHERE id = ?1 AND rider_user_id = ?2`,
		requestID, riderUserID).Scan(&pickupLat, &pickupLng, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrRiderRequestNotFound
	}
	if err != nil {
		return 0, err
	}
	if status != domain.RequestStatusPending {
		return 0, ErrRiderRequestConflictState
	}

	var confirmed int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(destination_confirmed, 0) FROM ride_requests WHERE id = ?1`, requestID).Scan(&confirmed); err != nil {
		confirmed = 0
	}
	if confirmed == 1 {
		return 0, ErrRiderRequestConflictState
	}

	estPrice := estimateRideRequestPrice(ctx, s.db, s.cfg, pickupLat, pickupLng, dropLat, dropLng)
	ttl := "+120 seconds"
	if s.cfg.RequestExpiresSeconds > 0 {
		ttl = fmt.Sprintf("+%d seconds", s.cfg.RequestExpiresSeconds)
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE ride_requests
		SET drop_lat = ?1, drop_lng = ?2, drop_name = ?3, estimated_price = ?4, expires_at = datetime('now', ?5)
		WHERE id = ?6 AND rider_user_id = ?7 AND status = ?8`,
		dropLat, dropLng, strings.TrimSpace(dropName), estPrice, ttl, requestID, riderUserID, domain.RequestStatusPending)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, ErrRiderRequestConflictState
	}
	return estPrice, nil
}

// ConfirmRequest mirrors rider bot handleRequestConfirmCallback: require
// estimate + non-expired PENDING, set destination_confirmed = 1, then
// MatchService.BroadcastRequest (same dispatch path as Telegram).
func (s *RiderRequestAppService) ConfirmRequest(ctx context.Context, riderUserID int64, requestID string) error {
	if s == nil || s.db == nil {
		return ErrRiderRequestMatchUnavailable
	}
	if s.match == nil {
		return ErrRiderRequestMatchUnavailable
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || riderUserID <= 0 {
		return ErrRiderRequestNotFound
	}

	var est int64
	var st string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(estimated_price, 0), status
		FROM ride_requests
		WHERE id = ?1 AND rider_user_id = ?2 AND expires_at > datetime('now')`,
		requestID, riderUserID).Scan(&est, &st)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRiderRequestNotFound
	}
	if err != nil {
		return err
	}
	if st != domain.RequestStatusPending || est <= 0 {
		return ErrRiderRequestConflictState
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE ride_requests SET destination_confirmed = 1
		WHERE id = ?1 AND rider_user_id = ?2 AND status = ?3`,
		requestID, riderUserID, domain.RequestStatusPending)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRiderRequestConflictState
	}

	if err := s.match.BroadcastRequest(ctx, requestID); err != nil {
		log.Printf("rider_request_app: broadcast request: %v", err)
	}
	return nil
}

// estimateRideRequestPrice is the same algorithm as internal/bot/rider.estimatePrice
// (OSRM route distance when available, else Haversine; FareService tiers, else config).
func estimateRideRequestPrice(ctx context.Context, db *sql.DB, cfg *config.Config, pickupLat, pickupLng, dropLat, dropLng float64) int64 {
	distanceKm := 0.0
	if route, err := GetRouteDistance(pickupLat, pickupLng, dropLat, dropLng); err == nil && route != nil && route.DistanceMeters > 0 {
		distanceKm = route.DistanceMeters / 1000
	} else {
		distanceKm = utils.HaversineMeters(pickupLat, pickupLng, dropLat, dropLng) / 1000
	}
	if distanceKm < 0 {
		distanceKm = 0
	}
	fareSvc := NewFareService(db, cfg)
	if fareSvc != nil {
		if v, err := fareSvc.CalculateFare(ctx, distanceKm); err == nil && v > 0 {
			return v
		}
	}
	startingFee := 4000
	pricePerKm := 1500
	if cfg != nil {
		startingFee = cfg.StartingFee
		pricePerKm = cfg.PricePerKm
	}
	return utils.CalculateFareRounded(float64(startingFee), float64(pricePerKm), distanceKm)
}
