package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

var (
	ErrNotFound     = errors.New("environment type not found")
	ErrConflictName = errors.New("environment type name already exists")
	ErrConflictCode = errors.New("environment type code already exists")
	ErrValidation   = errors.New("validation error")
)

type environmentTypeUseCase struct {
	repo port.EnvironmentTypeRepository
}

// NewEnvironmentTypeUseCase initializes a new EnvironmentTypeUseCase.
func NewEnvironmentTypeUseCase(repo port.EnvironmentTypeRepository) port.EnvironmentTypeUseCase {
	return &environmentTypeUseCase{
		repo: repo,
	}
}

func (u *environmentTypeUseCase) List(ctx context.Context) ([]model.EnvironmentType, error) {
	items, err := u.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.EnvironmentType{}
	}
	return items, nil
}

func (u *environmentTypeUseCase) GetByID(ctx context.Context, id string) (*model.EnvironmentType, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, ErrNotFound
	}
	item, err := u.repo.FindByID(ctx, trimmedID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

func (u *environmentTypeUseCase) Create(ctx context.Context, cmd port.CreateEnvironmentTypeCommand) (*model.EnvironmentType, error) {
	entity := &model.EnvironmentType{
		ID:          newUUID(),
		Code:        strings.TrimSpace(cmd.Code),
		Name:        strings.TrimSpace(cmd.Name),
		Description: cmd.Description,
		CreatedAt:   time.Now().UTC(),
	}

	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	// Verify name uniqueness
	existingByName, err := u.repo.FindByName(ctx, entity.Name)
	if err != nil {
		return nil, err
	}
	if existingByName != nil {
		return nil, ErrConflictName
	}

	// Verify code uniqueness if code is supplied
	if entity.Code != "" {
		existingByCode, err := u.repo.FindByCode(ctx, entity.Code)
		if err != nil {
			return nil, err
		}
		if existingByCode != nil {
			return nil, ErrConflictCode
		}
	}

	if err := u.repo.Save(ctx, entity); err != nil {
		if errors.Is(err, ErrConflictName) || errors.Is(err, ErrConflictCode) {
			return nil, err
		}
		return nil, err
	}

	return entity, nil
}

// newUUID generates a RFC4122 v4 UUID using crypto/rand.
func newUUID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40 // Version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
