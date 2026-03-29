package models

// AdminDriverDTO is the admin-facing view of a driver with balance.
type AdminDriverDTO struct {
	DriverID           int64  `json:"driver_id"`
	Name               string `json:"name"`
	Phone              string `json:"phone"`
	CarModel           string `json:"car_model"`
	PlateNumber        string `json:"plate_number"`
	Balance            int64  `json:"balance"`
	TotalPaid          int64  `json:"total_paid"`
	Status             string `json:"status"`             // "ACTIVE" or "INACTIVE"
	VerificationStatus string `json:"verification_status"` // pending, approved, rejected
	// Active-version legal acceptances (dashboard terms columns).
	DriverTermsOK bool `json:"driver_terms_ok"`
	UserTermsOK   bool `json:"user_terms_ok"`
	PrivacyOK     bool `json:"privacy_ok"`
}

// AdminRiderDTO is the admin-facing view of a rider with legal flags.
type AdminRiderDTO struct {
	ID           int64  `json:"id"`
	TelegramID   int64  `json:"telegram_id"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	UserTermsOK  bool   `json:"user_terms_ok"`
	PrivacyOK    bool   `json:"privacy_ok"`
}

