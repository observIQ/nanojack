package nanojack_test

import (
	"log"

	"github.com/observiq/nanojack"
)

// To use nanojack with the standard library's log package, just pass it into
// the SetOutput function when your application starts.
func Example() {
	log.SetOutput(&nanojack.Logger{
		Filename:   "/var/log/myapp/foo.log",
		MaxLines:   5,
		MaxBackups: 3,
	})
}
