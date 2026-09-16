package dto

import (
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

type CreateReservationRequest struct {
	EnvironmentID   string  `json:"environment_id"`
	ReservationDate string  `json:"reservation_date"`
	StartTime       string  `json:"start_time"`
	EndTime         string  `json:"end_time"`
	Reason          *string `json:"reason"`
}

type ReservationResponse struct {
	ID              string  `json:"id"`
	EnvironmentID   string  `json:"environment_id"`
	ReservationDate string  `json:"reservation_date"`
	StartTime       string  `json:"start_time"`
	EndTime         string  `json:"end_time"`
	Reason          *string `json:"reason"`
	RequesterID     string  `json:"requester_id"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ReservationListResponse struct {
	Data       []ReservationResponse `json:"data"`
	Pagination PaginationResponse    `json:"pagination"`
}

func (r CreateReservationRequest) Command(requesterID string) (port.CreateReservationCommand, error) {
	reservationDate, err := time.Parse(time.DateOnly, r.ReservationDate)
	if err != nil {
		return port.CreateReservationCommand{}, err
	}
	startTime, err := model.NormalizeReservationClock(r.StartTime)
	if err != nil {
		return port.CreateReservationCommand{}, err
	}
	endTime, err := model.NormalizeReservationClock(r.EndTime)
	if err != nil {
		return port.CreateReservationCommand{}, err
	}
	return port.CreateReservationCommand{
		EnvironmentID:   r.EnvironmentID,
		ReservationDate: reservationDate,
		StartTime:       startTime,
		EndTime:         endTime,
		Reason:          r.Reason,
		RequesterID:     requesterID,
	}, nil
}

func FromReservationModel(r model.Reservation) ReservationResponse {
	return ReservationResponse{
		ID:              r.ID,
		EnvironmentID:   r.EnvironmentID,
		ReservationDate: r.ReservationDate.UTC().Format(time.DateOnly),
		StartTime:       r.StartTime,
		EndTime:         r.EndTime,
		Reason:          r.Reason,
		RequesterID:     r.RequesterID,
		Status:          r.Status,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       r.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func FromReservationPage(page port.ReservationPage) ReservationListResponse {
	items := make([]ReservationResponse, len(page.Items))
	for i, item := range page.Items {
		items[i] = FromReservationModel(item)
	}
	return ReservationListResponse{
		Data: items,
		Pagination: PaginationResponse{
			Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages,
		},
	}
}
