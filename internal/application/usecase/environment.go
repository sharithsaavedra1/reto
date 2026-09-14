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
	ErrEnvironmentNotFound      = errors.New("training environment not found")
	ErrInvalidEnvironmentType   = errors.New("referenced environment_type_id does not exist")
	ErrInvalidEnvironmentQuery  = errors.New("invalid training environment query")
	ErrInvalidEnvironmentUpdate = errors.New("at least one editable field is required")
)

type environmentUseCase struct {
	envRepo     port.EnvironmentRepository
	envTypeRepo port.EnvironmentTypeRepository
}

// NewEnvironmentUseCase initializes a new EnvironmentUseCase.
func NewEnvironmentUseCase(envRepo port.EnvironmentRepository, envTypeRepo port.EnvironmentTypeRepository) port.EnvironmentUseCase {
	return &environmentUseCase{
		envRepo:     envRepo,
		envTypeRepo: envTypeRepo,
	}
}

func (u *environmentUseCase) Create(ctx context.Context, cmd port.CreateEnvironmentCommand) (*model.Environment, error) {
	entity := &model.Environment{
		ID:                newUUID(),
		EnvironmentTypeID: strings.TrimSpace(cmd.EnvironmentTypeID),
		TrainingCenterID:  strings.TrimSpace(cmd.TrainingCenterID),
		Name:              strings.TrimSpace(cmd.Name),
		Capacity:          cmd.Capacity,
		Location:          cmd.Location,
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	// E3: Verify that the referenced environment_type exists
	if u.envTypeRepo != nil {
		envType, err := u.envTypeRepo.FindByID(ctx, entity.EnvironmentTypeID)
		if err != nil {
			return nil, err
		}
		if envType == nil {
			return nil, ErrInvalidEnvironmentType
		}
	}

	if err := u.envRepo.Save(ctx, entity); err != nil {
		if errors.Is(err, ErrInvalidEnvironmentType) {
			return nil, ErrInvalidEnvironmentType
		}
		return nil, err
	}

	return entity, nil
}

func (u *environmentUseCase) GetByID(ctx context.Context, id string) (*model.Environment, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, ErrEnvironmentNotFound
	}

	item, err := u.envRepo.FindByID(ctx, trimmedID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrEnvironmentNotFound
	}
	return item, nil
}

// Update changes the editable fields of an environment and preserves its identity
// and creation timestamp. PUT and PATCH share this partial-update behavior because
// the published EnvironmentUpdate contract has optional fields.
func (u *environmentUseCase) Update(ctx context.Context, id string, cmd port.UpdateEnvironmentCommand) (*model.Environment, error) {
	item, err := u.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	changed := false
	if cmd.EnvironmentTypeID != nil {
		item.EnvironmentTypeID = strings.TrimSpace(*cmd.EnvironmentTypeID)
		changed = true
	}
	if cmd.Name != nil {
		item.Name = strings.TrimSpace(*cmd.Name)
		changed = true
	}
	if cmd.Capacity != nil {
		item.Capacity = *cmd.Capacity
		changed = true
	}
	if cmd.LocationSet {
		item.Location = cmd.Location
		changed = true
	}
	if cmd.IsActive != nil {
		item.IsActive = *cmd.IsActive
		changed = true
	}
	if !changed {
		return nil, ErrInvalidEnvironmentUpdate
	}

	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	if cmd.EnvironmentTypeID != nil && u.envTypeRepo != nil {
		envType, err := u.envTypeRepo.FindByID(ctx, item.EnvironmentTypeID)
		if err != nil {
			return nil, err
		}
		if envType == nil {
			return nil, ErrInvalidEnvironmentType
		}
	}

	item.UpdatedAt = time.Now().UTC()
	if err := u.envRepo.Update(ctx, item); err != nil {
		if errors.Is(err, ErrInvalidEnvironmentType) {
			return nil, ErrInvalidEnvironmentType
		}
		return nil, err
	}
	return item, nil
}

// Deactivate performs an idempotent logical deletion. The row remains in storage.
func (u *environmentUseCase) Deactivate(ctx context.Context, id string) error {
	item, err := u.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !item.IsActive {
		return nil
	}
	return u.envRepo.Deactivate(ctx, item.ID)
}

func (u *environmentUseCase) List(ctx context.Context) ([]model.Environment, error) {
	items, err := u.envRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.Environment{}
	}
	return items, nil
}

// Query returns a filtered, sorted and offset-paginated page of environments.
func (u *environmentUseCase) Query(ctx context.Context, query port.EnvironmentListQuery) (*port.EnvironmentPage, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return nil, ErrInvalidEnvironmentQuery
	}

	if query.EnvironmentTypeID != "" && !model.IsValidUUID(query.EnvironmentTypeID) {
		return nil, ErrInvalidEnvironmentQuery
	}
	if query.TrainingCenterID != "" && !model.IsValidUUID(query.TrainingCenterID) {
		return nil, ErrInvalidEnvironmentQuery
	}

	sortFields := map[string]struct{}{
		"id": {}, "name": {}, "capacity": {}, "created_at": {}, "updated_at": {}, "is_active": {},
	}
	if query.SortField == "" {
		query.SortField = "created_at"
	}
	if _, ok := sortFields[query.SortField]; !ok {
		return nil, ErrInvalidEnvironmentQuery
	}

	windowParts := 0
	if query.AvailableDate != nil {
		windowParts++
	}
	if query.AvailableStart != nil {
		windowParts++
	}
	if query.AvailableEnd != nil {
		windowParts++
	}
	if windowParts != 0 && windowParts != 3 {
		return nil, ErrInvalidEnvironmentQuery
	}
	if windowParts == 3 && !query.AvailableStart.Before(*query.AvailableEnd) {
		return nil, ErrInvalidEnvironmentQuery
	}

	return u.envRepo.Query(ctx, query)
}
