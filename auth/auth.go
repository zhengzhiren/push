package auth

type AuthChecker interface {
	Auth(token string) (bool, string)
}

var (
	Instance AuthChecker = nil
)

func NewInstance(provider string) bool {
	if provider == "letv" {
		Instance = newLetvAuth() 
		return true
	}
	return false
}


