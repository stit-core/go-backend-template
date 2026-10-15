package user

type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

type EnsureAdminInput struct {
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
