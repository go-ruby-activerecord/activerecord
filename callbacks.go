// Copyright (c) the go-ruby-activerecord/activerecord authors
//
// SPDX-License-Identifier: BSD-3-Clause

package activerecord

// This file implements ActiveRecord's callback lifecycle: the before/after hooks
// that run around validation, save, create, update and destroy, plus the
// transactional after_commit / after_rollback hooks. It mirrors
// ActiveSupport::Callbacks' ordering and halting semantics exactly:
//
//   - before_* callbacks run in the order they were registered; an after_*
//     callback runs in the reverse order (a last-in-first-out unwinding), which
//     is what ActiveRecord's define_model_callbacks produces.
//   - a before_* callback that returns [ErrAbort] (ActiveRecord's `throw :abort`)
//     halts the chain: no later callback in the same phase, and no wrapped
//     operation, runs, and the originating save/create/update/destroy returns
//     false with no error (matching AR, where a halted callback makes save
//     return false rather than raise).
//   - a callback that returns any other (non-nil) error aborts the operation and
//     the error propagates, rolling back the surrounding transaction.
//
// The callback bodies themselves run in the host (`rbgo`), but the registration,
// ordering, halting and firing are deterministic and live here so a differential
// oracle can compare the observable ordering to ActiveRecord's.

import "errors"

// ErrAbort is the sentinel a before_* callback returns to halt the chain, the
// pure-Go equivalent of ActiveRecord's `throw :abort`. A halted lifecycle makes
// the triggering persistence method return (false, nil): a soft failure, not an
// error.
var ErrAbort = errors.New("activerecord: callback chain halted")

// callbackKind enumerates the lifecycle phases a callback can hook.
type callbackKind int

const (
	beforeValidationCB callbackKind = iota
	afterValidationCB
	beforeSaveCB
	afterSaveCB
	beforeCreateCB
	afterCreateCB
	beforeUpdateCB
	afterUpdateCB
	beforeDestroyCB
	afterDestroyCB
	afterCommitCB
	afterRollbackCB
	numCallbackKinds
)

// Callback is a lifecycle hook body. It receives the record and returns nil to
// continue, [ErrAbort] to halt a before_* chain (soft failure), or any other
// error to abort the operation with that error.
type Callback func(*Record) error

// registerCallback appends fn to a phase's list, lazily allocating the store.
func (m *Model) registerCallback(k callbackKind, fn Callback) *Model {
	if m.callbacks == nil {
		m.callbacks = make([][]Callback, numCallbackKinds)
	}
	m.callbacks[k] = append(m.callbacks[k], fn)
	return m
}

// BeforeValidation registers a before_validation callback.
func (m *Model) BeforeValidation(fn Callback) *Model {
	return m.registerCallback(beforeValidationCB, fn)
}

// AfterValidation registers an after_validation callback.
func (m *Model) AfterValidation(fn Callback) *Model { return m.registerCallback(afterValidationCB, fn) }

// BeforeSave registers a before_save callback (runs for both create and update).
func (m *Model) BeforeSave(fn Callback) *Model { return m.registerCallback(beforeSaveCB, fn) }

// AfterSave registers an after_save callback.
func (m *Model) AfterSave(fn Callback) *Model { return m.registerCallback(afterSaveCB, fn) }

// BeforeCreate registers a before_create callback.
func (m *Model) BeforeCreate(fn Callback) *Model { return m.registerCallback(beforeCreateCB, fn) }

// AfterCreate registers an after_create callback.
func (m *Model) AfterCreate(fn Callback) *Model { return m.registerCallback(afterCreateCB, fn) }

// BeforeUpdate registers a before_update callback.
func (m *Model) BeforeUpdate(fn Callback) *Model { return m.registerCallback(beforeUpdateCB, fn) }

// AfterUpdate registers an after_update callback.
func (m *Model) AfterUpdate(fn Callback) *Model { return m.registerCallback(afterUpdateCB, fn) }

// BeforeDestroy registers a before_destroy callback.
func (m *Model) BeforeDestroy(fn Callback) *Model { return m.registerCallback(beforeDestroyCB, fn) }

// AfterDestroy registers an after_destroy callback.
func (m *Model) AfterDestroy(fn Callback) *Model { return m.registerCallback(afterDestroyCB, fn) }

// AfterCommit registers an after_commit callback (runs once the outermost
// transaction commits).
func (m *Model) AfterCommit(fn Callback) *Model { return m.registerCallback(afterCommitCB, fn) }

// AfterRollback registers an after_rollback callback (runs when the transaction
// rolls back).
func (m *Model) AfterRollback(fn Callback) *Model { return m.registerCallback(afterRollbackCB, fn) }

// runBefore fires a before_* phase in registration order. It stops at the first
// callback that returns a non-nil error and returns that error (ErrAbort for a
// soft halt).
func (m *Model) runBefore(k callbackKind, rec *Record) error {
	if m.callbacks == nil {
		return nil
	}
	for _, cb := range m.callbacks[k] {
		if err := cb(rec); err != nil {
			return err
		}
	}
	return nil
}

// runAfter fires an after_* phase in reverse registration order (LIFO), matching
// ActiveSupport::Callbacks. after_* callbacks do not halt the chain, but a
// non-nil error still aborts the operation (rolling back the transaction), as in
// ActiveRecord.
func (m *Model) runAfter(k callbackKind, rec *Record) error {
	if m.callbacks == nil {
		return nil
	}
	list := m.callbacks[k]
	for i := len(list) - 1; i >= 0; i-- {
		if err := list[i](rec); err != nil {
			return err
		}
	}
	return nil
}
