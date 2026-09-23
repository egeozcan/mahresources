package jobs

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// This file holds the Kind registry: the one place that knows which kinds of
// work this process can run, what each of them declares about itself, and how to
// reach its executor.
//
// It is deliberately keyed on the (Kind, version) pair rather than on the Kind
// alone. A Kind version is a version of *input semantics* — the same key the
// replay codecs are registered under — so two versions of one Kind are two
// adapters, and a Job accepted under a version this release cannot run is
// refused rather than handed to the adapter that happens to share its Kind.

// Adapter is one Kind's executor: the declaration that fixes how its work is
// dispatched, and the hooks the control plane calls to run it, to reconcile what
// a dead claimant left behind, and to offer and run its controls.
//
// An adapter requests lifecycle changes through the Execution (or
// ReconcileRequest) it was handed; it never writes lifecycle rows itself. That
// is what keeps every accepted transition, its event and its fencing check in
// one transaction, whichever executor asked for it.
type Adapter interface {
	// Definition fixes what the control plane must know about the Kind before it
	// claims any of its work.
	Definition() Definition
	// Dispatch runs one claimed execution. It returns when the execution has
	// ended — having reported the outcome through the Execution it was handed —
	// and returning while the Job is still running means the runtime ends the
	// execution on its behalf.
	Dispatch(context.Context, Execution) error
	// Reconcile answers what should happen to one Job whose claim expired. It is
	// asked rather than told because only the Kind knows whether the external
	// work the claim started is still running.
	Reconcile(context.Context, ReconcileRequest) (ReconcileDecision, error)
	// CleanupArtifacts removes, or confirms the absence of, the artifacts one
	// expired Job published, before its history is pruned. An adapter that
	// publishes no artifacts never sees one of these; an adapter that does is
	// the only thing that can say whether one is really gone.
	CleanupArtifacts(context.Context, ArtifactCleanupRequest) (ArtifactCleanupResult, error)
	// Commands reports the controls one Job offers right now, under the asking
	// principal's current access.
	Commands(context.Context, CommandContext) ([]Command, error)
	// ExecuteCommand runs one advertised control.
	ExecuteCommand(context.Context, CommandExecution) (CommandOutcome, error)
}

// HostTransitionAdapter lets a Kind keep its durable source row in step when
// the host applies a control directly to waiting work that has no executor to
// notify. It runs inside the same transaction as the canonical transition.
type HostTransitionAdapter interface {
	ApplyHostTransition(context.Context, Deps, Snapshot, string, State) error
}

// HostTransitionCompletion lets a Kind release its in-memory admission after a
// host-applied waiting-work transition has committed.
type HostTransitionCompletion interface {
	AfterHostTransition(context.Context, Snapshot, string, State)
}

// AdapterRegistration pairs a Kind's fixed definition with the adapter that runs
// it. The definition is the one captured at registration, so a runtime reading
// its budgets cannot be surprised by an adapter that answers differently on a
// later call.
type AdapterRegistration struct {
	Definition Definition
	Adapter    Adapter
}

// kindVersion names one registered adapter: a Kind and one version of its input
// semantics.
type kindVersion struct {
	kind    string
	version uint
}

// RegisterAdapter teaches the Service to run one Kind version.
//
// Registration is per (Kind, version) and a repeated pair is refused rather than
// replaced, for the same reason the replay codecs are: two adapters for one
// version means two executors for the same work, and which one ran would depend
// on initialization order.
func (s *Service) RegisterAdapter(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("%w: no adapter was supplied", ErrInvalidDefinition)
	}
	definition := adapter.Definition()
	if err := validateDefinition(definition); err != nil {
		return err
	}
	if definition.Visibility == "" {
		definition.Visibility = VisibilityOwner
	}

	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.adapters == nil {
		s.adapters = map[kindVersion]AdapterRegistration{}
	}
	key := kindVersion{kind: definition.Kind, version: definition.KindVersion}
	if _, exists := s.adapters[key]; exists {
		return fmt.Errorf("%w: %s v%d is already registered", ErrInvalidDefinition, definition.Kind, definition.KindVersion)
	}
	s.adapters[key] = AdapterRegistration{Definition: definition, Adapter: adapter}
	return nil
}

// Registrations returns every registered adapter with the definition it
// registered under, ordered by Kind and version so a runtime's pass over them is
// stable.
func (s *Service) Registrations() []AdapterRegistration {
	s.adapterMu.Lock()
	registered := make([]AdapterRegistration, 0, len(s.adapters))
	for _, registration := range s.adapters {
		registered = append(registered, registration)
	}
	s.adapterMu.Unlock()

	sort.Slice(registered, func(i, j int) bool {
		if registered[i].Definition.Kind != registered[j].Definition.Kind {
			return registered[i].Definition.Kind < registered[j].Definition.Kind
		}
		return registered[i].Definition.KindVersion < registered[j].Definition.KindVersion
	})
	return registered
}

// adapterFor returns the adapter registered for one (Kind, version) pair, or
// ErrAdapterUnregistered. It is the refusal that keeps a runtime from falling
// back to whichever executor is nearest.
func (s *Service) adapterFor(kind string, version uint) (Adapter, Definition, error) {
	s.adapterMu.Lock()
	registration, ok := s.adapters[kindVersion{kind: kind, version: version}]
	s.adapterMu.Unlock()
	if !ok {
		return nil, Definition{}, fmt.Errorf("%w: %s v%d", ErrAdapterUnregistered, kind, version)
	}
	return registration.Adapter, registration.Definition, nil
}

// AdapterFor reports the adapter registered for a (Kind, version) pair.
//
// It is the lookup a runtime needs for work it did not claim itself: a resumed
// execution comes back from reconciliation already owned, and the caller that has
// to run it is not the one that claimed it.
func (s *Service) AdapterFor(kind string, version uint) (Adapter, bool) {
	adapter, _, err := s.adapterFor(kind, version)
	return adapter, err == nil
}

// validateDefinition checks a Kind's declaration before it is registered. The
// bounds are the same ones every other identity in this module carries, because
// a Kind key is stored on every Job and every claim of that Kind.
func validateDefinition(definition Definition) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidDefinition, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(definition.Kind) == "" {
		return invalid("kind is required")
	}
	if len(definition.Kind) > MaxKindBytes {
		return invalid("kind is %d bytes, over the %d-byte ceiling", len(definition.Kind), MaxKindBytes)
	}
	if definition.KindVersion == 0 {
		return invalid("%s must declare a kind version of at least 1", definition.Kind)
	}
	if len(definition.CapacityGroup) > MaxCapacityGroupBytes {
		return invalid("capacity group is %d bytes, over the %d-byte ceiling",
			len(definition.CapacityGroup), MaxCapacityGroupBytes)
	}
	if definition.MaxConcurrent < 0 {
		return invalid("max concurrent is negative")
	}
	if definition.Lease < 0 {
		return invalid("lease is negative")
	}
	switch definition.Visibility {
	case "":
		// The zero value is ordinary user-facing work, which is what most Kinds
		// are. An operator-only Kind declares VisibilityAdmin and is refused
		// every other spelling.
	default:
		if definition.Visibility != VisibilityOwner && definition.Visibility != VisibilityAdmin {
			return invalid("visibility %q is not one of %s, %s",
				definition.Visibility, VisibilityOwner, VisibilityAdmin)
		}
	}
	return nil
}
