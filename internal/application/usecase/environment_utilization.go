package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
)

var (
	ErrUtilizationRangeRequired = errors.New("from and to are required")
	ErrUtilizationRangeInvalid  = errors.New("from must be before to")
	ErrUtilizationPagination    = errors.New("invalid pagination")
	ErrUtilizationFilter        = errors.New("filters must be UUIDs")
)

type environmentUtilizationUseCase struct {
	repository port.EnvironmentUtilizationRepository
}

func NewEnvironmentUtilizationUseCase(repository port.EnvironmentUtilizationRepository) port.EnvironmentUtilizationUseCase {
	return &environmentUtilizationUseCase{repository: repository}
}

func (u *environmentUtilizationUseCase) Report(ctx context.Context, query port.EnvironmentUtilizationQuery) (*port.EnvironmentUtilizationPage, error) {
	if query.From.IsZero() || query.To.IsZero() {
		return nil, ErrUtilizationRangeRequired
	}
	if !query.From.Before(query.To) {
		return nil, ErrUtilizationRangeInvalid
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return nil, ErrUtilizationPagination
	}
	for _, value := range []string{query.TrainingCenterID, query.EnvironmentID, query.EnvironmentTypeID} {
		if value != "" && !looksLikeUUID(value) {
			return nil, ErrUtilizationFilter
		}
	}
	return u.repository.Report(ctx, query)
}

func looksLikeUUID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
