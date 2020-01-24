package dblab

import "fmt"

// Clone contains connection info of a clone.
type Clone struct {
	Name     string
	Host     string
	Port     string
	Username string
	Password string
	SSLMode  string
}

func (clone Clone) ConnectionString() string {
	return fmt.Sprintf("host=%q port=%q user=%q dbname=%q password=%q",
		clone.Host, clone.Port, clone.Username, clone.Name, clone.Password)
}
