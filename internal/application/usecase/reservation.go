package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

var (
	ErrReservationOverlap            = errors.New("reservation overlaps an existing active reservation")
	ErrReservationValidation         = errors.New("invalid reservation")
	ErrReservationInvalidEnvironment = errors.New("referenced environment_id does not exist")
)

type reservationUseCase struct {
	repo    port.ReservationRepository
	envRepo port.EnvironmentRepository
}

func NewReservationUseCase(repo port.ReservationRepository, envRepo port.EnvironmentRepository) port.ReservationUseCase {
	return &reservationUseCase{repo: repo, envRepo: envRepo}
}

func (u *reservationUseCase) Create(ctx context.Context, command port.CreateReservationCommand) (*model.Reservation, error) {
	startTime, err := model.NormalizeReservationClock(command.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, model.ErrInvalidReservationStartTime)
	}
	endTime, err := model.NormalizeReservationClock(command.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, model.ErrInvalidReservationEndTime)
	}

	now := time.Now().UTC()
	reservation := &model.Reservation{
		ID:              newUUID(),
		EnvironmentID:   strings.TrimSpace(command.EnvironmentID),
		ReservationDate: dateOnly(command.ReservationDate),
		StartTime:       startTime,
		EndTime:         endTime,
		Reason:          command.Reason,
		RequesterID:     strings.TrimSpace(command.RequesterID),
		Status:          model.ReservationStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := reservation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, err)
	}

	if u.envRepo != nil {
		environment, err := u.envRepo.FindByID(ctx, reservation.EnvironmentID)
		if err != nil {
			return nil, err
		}
		if environment == nil {
			return nil, ErrReservationInvalidEnvironment
		}
	}

	if err := u.repo.Create(ctx, reservation); err != nil {
		return nil, err
	}
	return reservation, nil
}
