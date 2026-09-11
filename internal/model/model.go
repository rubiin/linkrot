package model

// LinkCheck represents the result of checking a single URL.
type LinkCheck struct {
	URL           string        // the URL that was checked
	Status        int           // final HTTP status code, 0 if error or skipped
	Alive         bool          // true if final status is 2xx/3xx after following redirects
	RedirectChain []RedirectStep // intermediate redirect steps (empty if no redirect)
	Err           string        // non-HTTP error description, empty if successful
	SourceFile    string        // which input file contained this link
	Skipped       bool          // true when the URL's host matched --ignore-hosts
}

// RedirectStep is one hop in a redirect chain.
type RedirectStep struct {
	URL    string // the URL that was redirected to
	Status int    // the HTTP status code of the redirect response
}
