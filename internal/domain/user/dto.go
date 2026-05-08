package user

type Create struct {
	Email    string
	Password string
	Name     string
}

type Update struct {
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
