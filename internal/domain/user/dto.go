package user

type CreateInput struct {
	Email    string
	Password string
	Name     string
}

type UpdateInput struct {
	Name    *string
	Status  *Status
	Version int
}

type ListFilter struct {
	Status *Status
}

type ListPagination struct {
	Page int
	Size int
}
