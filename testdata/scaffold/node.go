package E

type I interface {
	meth()
}

type T1 struct{}

func (t T1) meth() {}

type T2 struct{}

func (t *T2) meth() {}

func f(i I) {
	switch i := i.(type) {
	default:
		_ = i
	}
}
