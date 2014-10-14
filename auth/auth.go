package auth

type AuthChecker interface {
	Auth(token string) (bool, string)
}

var (
	Instance AuthChecker = newLetvAuth()
)

