package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Usecase interface {
	Register(ctx context.Context, in *user.CreateInput) (*user.User, error)
	GetByID(ctx context.Context, id string) (*user.User, error)
	Me(ctx context.Context) (*user.User, error)
	List(ctx context.Context, filter user.ListFilter, page paging.Page) ([]*user.User, int, error)
	Update(ctx context.Context, id string, in user.UpdateInput) (*user.User, error)
	Delete(ctx context.Context, id string) error
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, registerErrors.Declare(httpkit.Public(huma.Operation{
		OperationID:   "registerUser",
		Method:        http.MethodPost,
		Path:          "/api/auth/register",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusCreated,
		Summary:       "Регистрация нового пользователя",
	})), h.register)

	huma.Register(api, meErrors.Declare(authgate.Secured(huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/users/me",
		Tags:        []string{"users"},
		Summary:     "Текущий пользователь",
	})), h.me)

	huma.Register(api, listErrors.Declare(authgate.Secured(huma.Operation{
		OperationID: "listUsers",
		Method:      http.MethodGet,
		Path:        "/api/users",
		Tags:        []string{"users"},
		Summary:     "Список пользователей (admin)",
	})), h.list)

	huma.Register(api, getErrors.Declare(authgate.Secured(huma.Operation{
		OperationID: "getUser",
		Method:      http.MethodGet,
		Path:        "/api/users/{id}",
		Tags:        []string{"users"},
		Summary:     "Получить пользователя по ID",
	})), h.getByID)

	huma.Register(api, updateErrors.Declare(authgate.Secured(huma.Operation{
		OperationID: "updateUser",
		Method:      http.MethodPatch,
		Path:        "/api/users/{id}",
		Tags:        []string{"users"},
		Summary:     "Обновить пользователя",
	})), h.update)

	huma.Register(api, deleteErrors.Declare(authgate.Secured(huma.Operation{
		OperationID:   "deleteUser",
		Method:        http.MethodDelete,
		Path:          "/api/users/{id}",
		Tags:          []string{"users"},
		Summary:       "Удалить пользователя (soft-delete)",
		DefaultStatus: http.StatusNoContent,
	})), h.delete)
}

func (h *Handler) register(ctx context.Context, in *UserRegisterInput) (*UserRegisterOutput, error) {
	u, err := h.uc.Register(ctx, &user.CreateInput{
		Email:    in.Body.Email,
		Password: in.Body.Password,
		Name:     in.Body.Name,
	})
	if err != nil {
		return nil, userErrors.Map(err)
	}
	out := &UserRegisterOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) getByID(ctx context.Context, in *UserGetInput) (*UserGetOutput, error) {
	u, err := h.uc.GetByID(ctx, in.ID)
	if err != nil {
		return nil, userErrors.Map(err)
	}
	out := &UserGetOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) me(ctx context.Context, _ *struct{}) (*UserMeOutput, error) {
	u, err := h.uc.Me(ctx)
	if err != nil {
		return nil, userErrors.Map(err)
	}
	out := &UserMeOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) list(ctx context.Context, in *UserListInput) (*UserListOutput, error) {
	filter := user.ListFilter{}
	if in.Status != "" {
		s := user.Status(in.Status)
		filter.Status = &s
	}

	list, total, err := h.uc.List(ctx, filter, in.Page)
	if err != nil {
		return nil, userErrors.Map(err)
	}

	out := &UserListOutput{}
	out.Body.Total = total
	out.Body.Page = in.Number
	out.Body.Size = in.Size
	out.Body.Data = make([]UserView, len(list))
	for i, u := range list {
		out.Body.Data[i].FromEntity(u)
	}
	return out, nil
}

func (h *Handler) update(ctx context.Context, in *UserUpdateInput) (*UserUpdateOutput, error) {
	u, err := h.uc.Update(ctx, in.ID, user.UpdateInput{
		Name:    in.Body.Name,
		Status:  in.Body.Status,
		Version: in.Body.Version,
	})
	if err != nil {
		return nil, userErrors.Map(err)
	}
	out := &UserUpdateOutput{}
	out.Body.Data.FromEntity(u)
	return out, nil
}

func (h *Handler) delete(ctx context.Context, in *UserDeleteInput) (*struct{}, error) {
	if err := h.uc.Delete(ctx, in.ID); err != nil {
		return nil, userErrors.Map(err)
	}
	return nil, nil
}

// Правило каждой ошибки домена — одно. userErrors переводит в хендлерах их все (ни одна не уходит
// 500 — archtest), таблица операции объявляет в OpenAPI те, что отдаёт её usecase.
var (
	notFoundRule         = httpkit.ErrorRule{Err: user.ErrNotFound, Status: http.StatusNotFound, Code: "USER_NOT_FOUND", Detail: "user not found"}
	alreadyExistsRule    = httpkit.ErrorRule{Err: user.ErrAlreadyExists, Status: http.StatusConflict, Code: "USER_ALREADY_EXISTS", Detail: "email already taken"}
	invalidEmailRule     = httpkit.ErrorRule{Err: user.ErrInvalidEmail, Status: http.StatusUnprocessableEntity, Code: "USER_INVALID_EMAIL", Detail: "invalid email", Field: "body.email"}
	invalidNameRule      = httpkit.ErrorRule{Err: user.ErrInvalidName, Status: http.StatusUnprocessableEntity, Code: "USER_INVALID_NAME", Detail: "name must not contain NUL", Field: "body.name"}
	invalidRoleRule      = httpkit.ErrorRule{Err: user.ErrInvalidRole, Status: http.StatusUnprocessableEntity, Code: "USER_INVALID_ROLE", Detail: "invalid role"}
	weakPasswordRule     = httpkit.ErrorRule{Err: user.ErrWeakPassword, Status: http.StatusUnprocessableEntity, Code: "USER_WEAK_PASSWORD", Detail: "password too weak", Field: "body.password"}
	passwordTooLongRule  = httpkit.ErrorRule{Err: user.ErrPasswordTooLong, Status: http.StatusUnprocessableEntity, Code: "USER_PASSWORD_TOO_LONG", Detail: "password too long", Field: "body.password"}
	concurrentUpdateRule = httpkit.ErrorRule{Err: user.ErrConcurrentUpdate, Status: http.StatusConflict, Code: "USER_CONCURRENT_UPDATE", Detail: "concurrent update detected"}
	noUpdateFieldsRule   = httpkit.ErrorRule{Err: user.ErrNoUpdateFields, Status: http.StatusBadRequest, Code: "USER_NO_UPDATE_FIELDS", Detail: "at least one of name or status must be provided"}
	statusTransitionRule = httpkit.ErrorRule{Err: user.ErrInvalidStatusTransition, Status: http.StatusConflict, Code: "USER_INVALID_STATUS_TRANSITION", Detail: "status transition is not allowed"}

	userErrors = httpkit.NewErrorMap(notFoundRule, alreadyExistsRule, invalidEmailRule, invalidNameRule, invalidRoleRule,
		weakPasswordRule, passwordTooLongRule, concurrentUpdateRule, noUpdateFieldsRule, statusTransitionRule, authgate.ForbiddenRule)

	registerErrors = httpkit.NewErrorMap(weakPasswordRule, passwordTooLongRule, invalidEmailRule, invalidNameRule, alreadyExistsRule)
	meErrors       = httpkit.NewErrorMap(notFoundRule, authgate.ForbiddenRule)
	listErrors     = httpkit.NewErrorMap(authgate.ForbiddenRule)
	getErrors      = httpkit.NewErrorMap(notFoundRule, authgate.ForbiddenRule)
	updateErrors   = httpkit.NewErrorMap(notFoundRule, noUpdateFieldsRule, invalidNameRule, concurrentUpdateRule, statusTransitionRule, authgate.ForbiddenRule)
	deleteErrors   = httpkit.NewErrorMap(notFoundRule, authgate.ForbiddenRule)
)
