package verification

const minPasswordLen = 8

func PasswordStrengthOK(plain string) bool { return len(plain) >= minPasswordLen }
