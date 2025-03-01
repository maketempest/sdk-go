package resource

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CanonicalOperation struct {
	Type        CanonicalType
	Priority    int
	Concurrency int
}

type CanonicalType int

// TODO: cleanup and deduplicate canonical operations
const (
	Install CanonicalType = iota
	Uninstall
	Create
	Read
	Update
	Delete
	Upgrade
	Rollback
	Destroy
	Configure
	Write
	List
	Get
	Test
	Sync
)

// String returns the string representation of a CanonicalType
func (ct CanonicalType) String() string {
	switch ct {
	case Install:
		return "install"
	case Uninstall:
		return "uninstall"
	case Create:
		return "create"
	case Read:
		return "read"
	case Update:
		return "update"
	case Delete:
		return "delete"
	case Upgrade:
		return "upgrade"
	case Rollback:
		return "rollback"
	case Destroy:
		return "destroy"
	case Configure:
		return "configure"
	case Write:
		return "write"
	case List:
		return "list"
	case Get:
		return "get"
	case Test:
		return "test"
	case Sync:
		return "sync"
	default:
		return ""
	}
}

// MarshalJSON implements json.Marshaler for CanonicalType
func (ct CanonicalType) MarshalJSON() ([]byte, error) {
	return json.Marshal(ct.String())
}

// UnmarshalJSON implements json.Unmarshaler for CanonicalType
func (ct *CanonicalType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	newCt, ok := StringToCanonicalType(s)
	if !ok {
		return fmt.Errorf("invalid canonical type: %s", s)
	}

	*ct = newCt
	return nil
}

// StringToCanonicalType converts a string to its CanonicalType equivalent
func StringToCanonicalType(s string) (CanonicalType, bool) {
	switch strings.ToLower(s) {
	case "install":
		return Install, true
	case "uninstall":
		return Uninstall, true
	case "create":
		return Create, true
	case "read":
		return Read, true
	case "update":
		return Update, true
	case "delete":
		return Delete, true
	case "upgrade":
		return Upgrade, true
	case "rollback":
		return Rollback, true
	case "destroy":
		return Destroy, true
	case "configure":
		return Configure, true
	case "write":
		return Write, true
	case "list":
		return List, true
	case "get":
		return Get, true
	case "test":
		return Test, true
	case "sync":
		return Sync, true
	default:
		return 0, false
	}
}

func (r *ResourceDefinition) Install(fn OperationFunc) {
	r.RegisterOperation("_install", fn, Op.On(CanonicalOperation{Type: Install}))
}

func (r *ResourceDefinition) Create(fn OperationFunc) {
	r.RegisterOperation("_create", fn, Op.On(CanonicalOperation{Type: Create}))
}

// ... other methods
func ExecuteOperation() {

	// Route to appropriate operation

}
