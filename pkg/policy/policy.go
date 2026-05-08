package policy

type Decision struct {
	IsAllowed bool
	Reason    string
}

func Allow() Decision             { return Decision{IsAllowed: true} }
func Deny(reason string) Decision { return Decision{IsAllowed: false, Reason: reason} }
