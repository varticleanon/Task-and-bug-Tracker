package db

import "errors"

var (
	ErrNotFound       = errors.New("resource not found or inaccessible")
	ErrConflict       = errors.New("version or unique constraint conflict")
	ErrTenantMismatch = errors.New("workspace isolation violation")
)
