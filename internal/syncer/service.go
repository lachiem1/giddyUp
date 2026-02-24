package syncer

import "context"

type Service struct {
	engine *Engine
}

func NewService(engine *Engine) *Service {
	return &Service{engine: engine}
}

func (s *Service) EnterAccountsView(ctx context.Context) error {
	return s.engine.EnterView(ctx, CollectionAccounts)
}

func (s *Service) LeaveView() {
	s.engine.LeaveView()
}

func (s *Service) RefreshAccounts() error {
	return s.engine.ManualRefresh(CollectionAccounts)
}

func (s *Service) EnterTransactionsView(ctx context.Context) error {
	return s.engine.EnterView(ctx, CollectionTransactions)
}

func (s *Service) RefreshTransactions() error {
	return s.engine.ManualRefresh(CollectionTransactions)
}
