package execute

import "github.com/onsi/ginkgo"

// BeforeAll gets executed before all the entries of
// the scope it's getting executed in.
func BeforeAll(fn func()) {
	first := true
	ginkgo.BeforeEach(func() {
		if first {
			first = false
			fn()
		}
	})
}

// AfterAll gets executed before all the entries of
// the scope it's getting executed in.
func AfterAll(fn func()) {
	first := true
	ginkgo.AfterEach(func() {
		if first {
			first = false
			fn()
		}
	})
}
