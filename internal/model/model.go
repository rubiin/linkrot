package model

// LinkCheck is the result of checking a single URL.
type LinkCheck struct {
	URL           string
	Status        int
	Alive         bool
	RedirectChain []RedirectStep
	Err           string
	SourceFile    string
}

// RedirectStep is one hop in a redirect chain.
type RedirectStep struct {
	URL    string
	Status int
}
