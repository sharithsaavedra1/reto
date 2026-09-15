package model_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

func TestMaintenanceValidate(t *testing.T) {
	valid := model.Maintenance{
		EnvironmentID: "78901234-5678-9012-3456-789012345678",
		StartDate:     time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		EndDate:       time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC),
		CreatedBy:     "42c78701-2b34-4a6f-b034-3f7a3eca57d8",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid maintenance should pass validation: %v", err)
	}

	cases := []struct {
		name string
		item model.Maintenance
		want error
	}{
		{"invalid environment", model.Maintenance{EnvironmentID: "invalid", StartDate: valid.StartDate, EndDate: valid.EndDate, CreatedBy: valid.CreatedBy}, model.ErrInvalidMaintenanceEnvironmentID},
		{"invalid creator", model.Maintenance{EnvironmentID: valid.EnvironmentID, StartDate: valid.StartDate, EndDate: valid.EndDate, CreatedBy: "user-123"}, model.ErrInvalidMaintenanceCreatedBy},
		{"end before start", model.Maintenance{EnvironmentID: valid.EnvironmentID, StartDate: valid.EndDate, EndDate: valid.StartDate, CreatedBy: valid.CreatedBy}, model.ErrInvalidMaintenanceDateRange},
		{"end equal start", model.Maintenance{EnvironmentID: valid.EnvironmentID, StartDate: valid.StartDate, EndDate: valid.StartDate, CreatedBy: valid.CreatedBy}, model.ErrInvalidMaintenanceDateRange},
		{"reason too long", model.Maintenance{EnvironmentID: valid.EnvironmentID, StartDate: valid.StartDate, EndDate: valid.EndDate, CreatedBy: valid.CreatedBy, Reason: stringPointer(strings.Repeat("r", 256))}, model.ErrMaintenanceReasonTooLong},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.item.Validate(); !errors.Is(err, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, err)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }
