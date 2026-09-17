package port

import "context"

type EnvironmentUtilizationRepository interface {
	Report(ctx context.Context, query EnvironmentUtilizationQuery) (*EnvironmentUtilizationPage, error)
}
