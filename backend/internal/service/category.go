package service

import (
	"context"
	"errors"

	"backend/internal/generated/ent"
	"backend/internal/repository"
)

var ErrInvalidParent = errors.New("invalid parent category")

type CategoryService interface {
	GetAll(ctx context.Context) ([]*ent.Category, error)
	GetByID(ctx context.Context, id string) (*ent.Category, error)
	GetActive(ctx context.Context) ([]*ent.Category, error)
	List(ctx context.Context, limit, offset int) ([]*ent.Category, int64, error)
	Create(ctx context.Context, name string, position int, parentID *string) (*ent.Category, error)
	Update(ctx context.Context, id string, name string, position int, isActive bool, parentID *string, setParent bool) (*ent.Category, error)
	Delete(ctx context.Context, id string) error
}

type categoryService struct {
	repo  repository.CategoryRepository
	cache *CatalogCache
}

func NewCategoryService(repo repository.CategoryRepository, cache *CatalogCache) CategoryService {
	return &categoryService{repo: repo, cache: cache}
}

func (s *categoryService) GetAll(ctx context.Context) ([]*ent.Category, error) {
	return s.repo.GetAll(ctx)
}

func (s *categoryService) GetByID(ctx context.Context, id string) (*ent.Category, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *categoryService) GetActive(ctx context.Context) ([]*ent.Category, error) {
	return s.repo.GetAllActive(ctx)
}

func (s *categoryService) List(ctx context.Context, limit, offset int) ([]*ent.Category, int64, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *categoryService) Create(ctx context.Context, name string, position int, parentID *string) (*ent.Category, error) {
	parentID = normalizeParentID(parentID)
	if err := s.validateParent(ctx, "", parentID); err != nil {
		return nil, err
	}
	created, err := s.repo.Create(ctx, name, position, true, parentID)
	if err != nil {
		return nil, err
	}
	s.invalidate()
	return created, nil
}

func (s *categoryService) Update(ctx context.Context, id string, name string, position int, isActive bool, parentID *string, setParent bool) (*ent.Category, error) {
	if setParent {
		parentID = normalizeParentID(parentID)
		if err := s.validateParent(ctx, id, parentID); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.Update(ctx, id, name, position, isActive, parentID, setParent)
	if err != nil {
		return nil, err
	}
	s.invalidate()
	return updated, nil
}

func normalizeParentID(parentID *string) *string {
	if parentID == nil || *parentID == "" {
		return nil
	}
	return parentID
}

func (s *categoryService) Delete(ctx context.Context, id string) error {
	children, err := s.repo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if children > 0 {
		return repository.ErrConflict
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *categoryService) validateParent(ctx context.Context, childID string, parentID *string) error {
	if parentID == nil || *parentID == "" {
		return nil
	}
	pid := *parentID
	if pid == childID {
		return ErrInvalidParent
	}
	parent, err := s.repo.GetByID(ctx, pid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidParent
		}
		return err
	}
	if parent.ParentID != nil {
		return ErrInvalidParent
	}
	if childID != "" {
		n, err := s.repo.CountChildren(ctx, childID)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrInvalidParent
		}
	}
	return nil
}

func (s *categoryService) invalidate() {
	if s.cache != nil {
		s.cache.invalidate()
	}
}
