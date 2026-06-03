package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-infra/golibinfra/httpkit"
	"git.stit.tech/stit-template/go-backend-template/internal/domain/user"
)

type Usecase interface {
	Register(ctx context.Context, in *user.Create) (*user.User, error)
	GetByID(ctx context.Context, id string) (*user.User, error)
	Me(ctx context.Context) (*user.User, error)
	List(ctx context.Context, filter user.ListFilter, pg user.ListPagination) ([]*user.User, int, error)
	Update(ctx context.Context, id string, in user.Update) (*user.User, error)
	Delete(ctx context.Context, id string) error
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, huma.Operation{
		OperationID:   "registerUser",
		Method:        http.MethodPost,
		Path:          "/api/auth/register",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusCreated,
		Summary:       "Регистрация нового пользователя",
	}, h.register)

	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/users/me",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"Cookie": {}}},
		Summary:     "Текущий пользователь",
	}, h.me)

	huma.Register(api, huma.Operation{
		OperationID: "listUsers",
		Method:      http.MethodGet,
		Path:        "/api/users",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"Cookie": {}}},
		Summary:     "Список пользователей (admin)",
	}, h.list)

	huma.Register(api, huma.Operation{
		OperationID: "getUser",
		Method:      http.MethodGet,
		Path:        "/api/users/{id}",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"Cookie": {}}},
		Summary:     "Получить пользователя по ID",
	}, h.getByID)

	huma.Register(api, huma.Operation{
		OperationID: "updateUser",
		Method:      http.MethodPatch,
		Path:        "/api/users/{id}",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"Cookie": {}}},
		Summary:     "Обновить пользователя",
	}, h.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteUser",
		Method:        http.MethodDelete,
		Path:          "/api/users/{id}",
		Tags:          []string{"users"},
		Security:      []map[string][]string{{"Cookie": {}}},
		Summary:       "Удалить пользователя (soft-delete)",
		DefaultStatus: http.StatusNoContent,
	}, h.delete)
}

func (h *Handler) register(ctx context.Context, in *RegisterInput) (*RegisterOutput, error) {
	u, err := h.uc.Register(ctx, in.ToEntity())
	if err != nil {
		return nil, mapErr(err)
	}
	out := &RegisterOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) getByID(ctx context.Context, in *GetUserInput) (*GetUserOutput, error) {
	u, err := h.uc.GetByID(ctx, in.ID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := &GetUserOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) me(ctx context.Context, _ *struct{}) (*MeOutput, error) {
	u, err := h.uc.Me(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := &MeOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) list(ctx context.Context, in *ListInput) (*ListOutput, error) {
	filter := user.ListFilter{}
	if in.Status != "" {
		s := user.Status(in.Status)
		filter.Status = &s
	}

	pg := user.ListPagination{
		Page: in.Page,
		Size: in.Size,
	}

	list, total, err := h.uc.List(ctx, filter, pg)
	if err != nil {
		return nil, mapErr(err)
	}

	out := &ListOutput{}
	out.Body.Total = total
	out.Body.Page = pg.Page
	out.Body.Size = pg.Size
	out.Body.Data = make([]UserView, len(list))
	for i, u := range list {
		out.Body.Data[i].FromEntity(u)
	}
	return out, nil
}

func (h *Handler) update(ctx context.Context, in *UpdateInput) (*UpdateOutput, error) {
	u, err := h.uc.Update(ctx, in.ID, user.Update{
		Name:    in.Body.Name,
		Status:  in.Body.Status,
		Version: in.Body.Version,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := &UpdateOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) delete(ctx context.Context, in *DeleteInput) (*struct{}, error) {
	if err := h.uc.Delete(ctx, in.ID); err != nil {
		return nil, mapErr(err)
	}
	return nil, nil
}

func mapErr(err error) error {
	return httpkit.MapErr(
		err,
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrAlreadyExists) },
			Code:   "USER_ALREADY_EXISTS",
			Status: http.StatusConflict,
			Detail: "email already taken",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrInvalidEmail) },
			Code:   "USER_INVALID_EMAIL",
			Status: http.StatusBadRequest,
			Detail: "invalid email",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrInvalidRole) },
			Code:   "USER_INVALID_ROLE",
			Status: http.StatusBadRequest,
			Detail: "invalid role",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrWeakPassword) },
			Code:   "USER_WEAK_PASSWORD",
			Status: http.StatusBadRequest,
			Detail: "password too weak",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrNotFound) },
			Code:   "USER_NOT_FOUND",
			Status: http.StatusNotFound,
			Detail: "user not found",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrConcurrentUpdate) },
			Code:   "USER_CONCURRENT_UPDATE",
			Status: http.StatusConflict,
			Detail: "concurrent update detected",
		},
		httpkit.ErrorMapping{
			Match:  func(e error) bool { return errors.Is(e, user.ErrNoUpdateFields) },
			Code:   "USER_NO_UPDATE_FIELDS",
			Status: http.StatusBadRequest,
			Detail: "at least one of name or status must be provided",
		},
		httpkit.ErrorMapping{
			Match: func(e error) bool {
				var fe *user.ForbiddenError
				return errors.As(e, &fe)
			},
			Code:   "FORBIDDEN",
			Status: http.StatusForbidden,
			Detail: "forbidden",
		},
	)
}
