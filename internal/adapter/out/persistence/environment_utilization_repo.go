package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

// MemoryEnvironmentUtilizationRepository reads the four domain repositories
// used by the physical utilization report. It intentionally has no write API.
type MemoryEnvironmentUtilizationRepository struct {
	environments port.EnvironmentRepository
	types        port.EnvironmentTypeRepository
	rules        port.AvailabilityRuleRepository
	maintenance  port.MaintenanceRepository
	reservations port.ReservationRepository
}

func NewMemoryEnvironmentUtilizationRepository(
	environments port.EnvironmentRepository,
	types port.EnvironmentTypeRepository,
	rules port.AvailabilityRuleRepository,
	maintenance port.MaintenanceRepository,
	reservations port.ReservationRepository,
) port.EnvironmentUtilizationRepository {
	return &MemoryEnvironmentUtilizationRepository{
		environments: environments,
		types:        types,
		rules:        rules,
		maintenance:  maintenance,
		reservations: reservations,
	}
}

func (r *MemoryEnvironmentUtilizationRepository) Report(ctx context.Context, query port.EnvironmentUtilizationQuery) (*port.EnvironmentUtilizationPage, error) {
	environments, err := r.environments.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]model.EnvironmentUtilization, 0, len(environments))
	for _, environment := range environments {
		if query.EnvironmentID != "" && environment.ID != query.EnvironmentID {
			continue
		}
		if query.TrainingCenterID != "" && environment.TrainingCenterID != query.TrainingCenterID {
			continue
		}
		if query.EnvironmentTypeID != "" && environment.EnvironmentTypeID != query.EnvironmentTypeID {
			continue
		}

		var typeCode string
		environmentType, err := r.types.FindByID(ctx, environment.EnvironmentTypeID)
		if err != nil {
			return nil, err
		}
		if environmentType != nil {
			typeCode = environmentType.Code
		}

		rules, err := r.rules.List(ctx, environment.ID, nil)
		if err != nil {
			return nil, err
		}
		from, to := query.From, query.To
		maintenance, err := r.maintenance.List(ctx, port.MaintenanceListQuery{EnvironmentID: environment.ID, From: &from, To: &to})
		if err != nil {
			return nil, err
		}
		reservations, err := queryAllReservations(ctx, r.reservations, environment.ID, from, to)
		if err != nil {
			return nil, err
		}

		available, maintenanceHours := calculateAvailableHours(rules, maintenance, query.From, query.To)
		reserved := calculateReservedHours(reservations)
		items = append(items, utilizationItem(environment, typeCode, query, available, reserved, maintenanceHours))
	}

	return paginateUtilization(items, query), nil
}

// PostgresEnvironmentUtilizationRepository performs the aggregation in SQL,
// using only the training_environment tables required by HU-ENV-010.
type PostgresEnvironmentUtilizationRepository struct {
	db *sql.DB
}

func NewPostgresEnvironmentUtilizationRepository(db *sql.DB) port.EnvironmentUtilizationRepository {
	return &PostgresEnvironmentUtilizationRepository{db: db}
}

func (r *PostgresEnvironmentUtilizationRepository) Report(ctx context.Context, query port.EnvironmentUtilizationQuery) (*port.EnvironmentUtilizationPage, error) {
	countWhere := []string{"1 = 1"}
	dataWhere := []string{"1 = 1"}
	filterArgs := make([]interface{}, 0, 3)
	addFilter := func(clause string, value string) {
		filterArgs = append(filterArgs, value)
		index := len(filterArgs)
		countWhere = append(countWhere, fmt.Sprintf(clause, index))
		dataWhere = append(dataWhere, fmt.Sprintf(clause, index+2))
	}
	if query.TrainingCenterID != "" {
		addFilter("e.training_center_id = $%d", query.TrainingCenterID)
	}
	if query.EnvironmentID != "" {
		addFilter("e.id = $%d", query.EnvironmentID)
	}
	if query.EnvironmentTypeID != "" {
		addFilter("e.environment_type_id = $%d", query.EnvironmentTypeID)
	}
	countWhereSQL := strings.Join(countWhere, " AND ")
	dataWhereSQL := strings.Join(dataWhere, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM training_environment.environment e
		WHERE `+countWhereSQL, filterArgs...).Scan(&total); err != nil {
		return nil, err
	}

	dataArgs := []interface{}{query.From.UTC().Format(time.DateOnly), query.To.UTC().Format(time.DateOnly)}
	dataArgs = append(dataArgs, filterArgs...)
	dataArgs = append(dataArgs, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.training_center_id, COALESCE(et.code, ''),
		       COALESCE((
				SELECT SUM(EXTRACT(EPOCH FROM (ar.end_time - ar.start_time)) / 3600.0)
				FROM generate_series($1::date, ($2::date - 1), interval '1 day') AS d(day)
				JOIN training_environment.availability_rule ar
				  ON ar.environment_id = e.id
				 AND ar.day_of_week = EXTRACT(ISODOW FROM d.day)::smallint
				 AND ar.effective_from <= d.day::date
				 AND (ar.effective_until IS NULL OR ar.effective_until >= d.day::date)
		       ), 0),
		       COALESCE((
				SELECT SUM(EXTRACT(EPOCH FROM (rsv.end_time - rsv.start_time)) / 3600.0)
				FROM training_environment.reservation rsv
				WHERE rsv.environment_id = e.id
				  AND rsv.reservation_date >= $1::date
				  AND rsv.reservation_date < $2::date
				  AND rsv.status <> 'CANCELLED'
		       ), 0),
		       COALESCE((
				SELECT SUM(EXTRACT(EPOCH FROM (ar.end_time - ar.start_time)) / 3600.0)
				FROM generate_series($1::date, ($2::date - 1), interval '1 day') AS d(day)
				JOIN training_environment.availability_rule ar
				  ON ar.environment_id = e.id
				 AND ar.day_of_week = EXTRACT(ISODOW FROM d.day)::smallint
				 AND ar.effective_from <= d.day::date
				 AND (ar.effective_until IS NULL OR ar.effective_until >= d.day::date)
				WHERE EXISTS (
					SELECT 1
					FROM training_environment.maintenance m
					WHERE m.environment_id = e.id
					  AND m.start_date <= d.day::date
					  AND m.end_date > d.day::date
				)
		       ), 0)
		FROM training_environment.environment e
		JOIN training_environment.environment_type et ON et.id = e.environment_type_id
		WHERE `+dataWhereSQL+`
		ORDER BY e.created_at ASC, e.id ASC
		LIMIT $`+strconv.Itoa(len(dataArgs)-1)+` OFFSET $`+strconv.Itoa(len(dataArgs)), dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.EnvironmentUtilization, 0)
	for rows.Next() {
		var item model.EnvironmentUtilization
		if err := rows.Scan(&item.EnvironmentID, &item.EnvironmentName, &item.TrainingCenterID,
			&item.EnvironmentTypeCode, &item.AvailableHours, &item.ReservedHours, &item.MaintenanceHours); err != nil {
			return nil, err
		}
		item.PeriodFrom, item.PeriodTo = query.From, query.To
		item.UtilizationPercentage = utilizationPercentage(item.ReservedHours, item.AvailableHours)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &port.EnvironmentUtilizationPage{
		Items: items, Page: query.Page, PageSize: query.PageSize,
		TotalItems: total, TotalPages: totalPages(total, query.PageSize),
	}, nil
}

func calculateAvailableHours(rules []model.AvailabilityRule, maintenance []model.Maintenance, from, to time.Time) (float64, float64) {
	var available, maintenanceHours float64
	for day := utilizationDateOnly(from); day.Before(to); day = day.AddDate(0, 0, 1) {
		weekday := int(day.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		for _, rule := range rules {
			if rule.DayOfWeek != weekday || rule.EffectiveFrom.IsZero() || utilizationDateOnly(rule.EffectiveFrom).After(day) || rule.EffectiveUntil != nil && !day.Before(utilizationDateOnly(*rule.EffectiveUntil).AddDate(0, 0, 1)) {
				continue
			}
			hours := clockHours(rule.StartTime, rule.EndTime)
			available += hours
			for _, period := range maintenance {
				if !day.Before(utilizationDateOnly(period.StartDate)) && day.Before(utilizationDateOnly(period.EndDate)) {
					maintenanceHours += hours
					break
				}
			}
		}
	}
	return available, maintenanceHours
}

func calculateReservedHours(reservations []model.Reservation) float64 {
	var total float64
	for _, reservation := range reservations {
		if reservation.Status == model.ReservationStatusCancelled {
			continue
		}
		start, startErr := model.ParseReservationClock(reservation.StartTime)
		end, endErr := model.ParseReservationClock(reservation.EndTime)
		if startErr == nil && endErr == nil && end > start {
			total += (end - start).Hours()
		}
	}
	return total
}

func utilizationItem(environment model.Environment, typeCode string, query port.EnvironmentUtilizationQuery, available, reserved, maintenance float64) model.EnvironmentUtilization {
	return model.EnvironmentUtilization{
		EnvironmentID: environment.ID, EnvironmentName: environment.Name,
		TrainingCenterID: environment.TrainingCenterID, EnvironmentTypeCode: typeCode,
		PeriodFrom: query.From, PeriodTo: query.To, AvailableHours: available,
		ReservedHours: reserved, MaintenanceHours: maintenance,
		UtilizationPercentage: utilizationPercentage(reserved, available),
	}
}

func utilizationPercentage(reserved, available float64) float64 {
	if available <= 0 {
		return 0
	}
	return reserved / available * 100
}

func paginateUtilization(items []model.EnvironmentUtilization, query port.EnvironmentUtilizationQuery) *port.EnvironmentUtilizationPage {
	start := (query.Page - 1) * query.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return &port.EnvironmentUtilizationPage{
		Items: items[start:end], Page: query.Page, PageSize: query.PageSize,
		TotalItems: len(items), TotalPages: totalPages(len(items), query.PageSize),
	}
}

func totalPages(total, pageSize int) int {
	if total == 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}

func utilizationDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func clockHours(start, end time.Time) float64 {
	startClock := time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute + time.Duration(start.Second())*time.Second
	endClock := time.Duration(end.Hour())*time.Hour + time.Duration(end.Minute())*time.Minute + time.Duration(end.Second())*time.Second
	if endClock <= startClock {
		return 0
	}
	return (endClock - startClock).Hours()
}

func queryAllReservations(ctx context.Context, repository port.ReservationRepository, environmentID string, from, to time.Time) ([]model.Reservation, error) {
	result := make([]model.Reservation, 0)
	for page := 1; ; page++ {
		current, err := repository.Query(ctx, port.ReservationListQuery{
			Page: page, PageSize: 100, EnvironmentID: environmentID, From: &from, To: &to,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, current.Items...)
		if page >= current.TotalPages || len(current.Items) == 0 {
			return result, nil
		}
	}
}
