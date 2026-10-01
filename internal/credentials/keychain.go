package credentials

import "errors"

// errMissing is every platform's "the item is not there".
var errMissing = errors.New("keychain item not found")

// errBlocked is the keychain wanting to ask the user something first. It
// is separate from absent because it has its own remedy: an item stored
// by another program — or by an earlier build of this one — carries an
// access control that does not name this binary.
var errBlocked = errors.New("the keychain did not authorize exigo-cli for this item")

// ui says whether a keychain call may put the OS authorization dialog on
// screen. The zero value refuses: a dialog nobody can see blocks forever,
// which is exactly the scripted, non-interactive case.
type ui bool

const (
	noUI    ui = false
	allowUI ui = true
)
