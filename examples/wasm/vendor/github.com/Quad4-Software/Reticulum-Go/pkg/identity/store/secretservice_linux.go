// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package store

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	ssDest      = "org.freedesktop.secrets"
	ssPath      = "/org/freedesktop/secrets"
	ssIface     = "org.freedesktop.Secret.Service"
	ssItemIface = "org.freedesktop.Secret.Item"
	ssCollIface = "org.freedesktop.Secret.Collection"
	ssCollPath  = "/org/freedesktop/secrets/aliases/default"
)

type ssSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// SecretServiceBackend stores secrets via Freedesktop Secret Service (D-Bus).
type SecretServiceBackend struct{}

func NewSecretServiceBackend() (*SecretServiceBackend, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLocked, err)
	}
	defer conn.Close()
	obj := conn.Object(ssDest, ssPath)
	var output dbus.Variant
	var sessionPath dbus.ObjectPath
	if err := obj.Call(ssIface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &sessionPath); err != nil {
		return nil, fmt.Errorf("%w: OpenSession: %v", ErrLocked, err)
	}
	return &SecretServiceBackend{}, nil
}

func (SecretServiceBackend) withSession(fn func(conn *dbus.Conn, session dbus.ObjectPath) error) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLocked, err)
	}
	defer conn.Close()
	obj := conn.Object(ssDest, ssPath)
	var output dbus.Variant
	var sessionPath dbus.ObjectPath
	if err := obj.Call(ssIface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &sessionPath); err != nil {
		return fmt.Errorf("%w: OpenSession: %v", ErrLocked, err)
	}
	defer func() {
		_ = conn.Object(ssDest, sessionPath).Call("org.freedesktop.Secret.Session.Close", 0)
	}()
	return fn(conn, sessionPath)
}

func (b SecretServiceBackend) Get(attrs map[string]string) ([]byte, error) {
	var secret []byte
	err := b.withSession(func(conn *dbus.Conn, session dbus.ObjectPath) error {
		svc := conn.Object(ssDest, ssPath)
		var unlocked, locked []dbus.ObjectPath
		if err := svc.Call(ssIface+".SearchItems", 0, attrs).Store(&unlocked, &locked); err != nil {
			return err
		}
		items := unlocked
		if len(items) == 0 && len(locked) > 0 {
			var prompted dbus.ObjectPath
			var unlockedPaths []dbus.ObjectPath
			if err := svc.Call(ssIface+".Unlock", 0, locked).Store(&unlockedPaths, &prompted); err != nil {
				return fmt.Errorf("%w: Unlock: %v", ErrLocked, err)
			}
			items = unlockedPaths
			if len(items) == 0 {
				return ErrLocked
			}
		}
		if len(items) == 0 {
			return ErrNotFound
		}
		item := conn.Object(ssDest, items[0])
		var sec ssSecret
		if err := item.Call(ssItemIface+".GetSecret", 0, session).Store(&sec); err != nil {
			return err
		}
		secret = append([]byte(nil), sec.Value...)
		return nil
	})
	return secret, err
}

func (b SecretServiceBackend) Set(attrs map[string]string, secret []byte, label string) error {
	if label == "" {
		label = "reticulum-go identity"
		if p := attrs[AttrIdentityPath]; p != "" {
			label = "reticulum-go identity " + p
		}
	}
	return b.withSession(func(conn *dbus.Conn, session dbus.ObjectPath) error {
		coll := conn.Object(ssDest, ssCollPath)
		props := map[string]dbus.Variant{
			"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant(label),
			"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(attrs),
		}
		sec := ssSecret{
			Session:     session,
			Parameters:  []byte{},
			Value:       secret,
			ContentType: "application/octet-stream",
		}
		var itemPath, prompt dbus.ObjectPath
		if err := coll.Call(ssCollIface+".CreateItem", 0, props, sec, true).Store(&itemPath, &prompt); err != nil {
			return fmt.Errorf("CreateItem: %w", err)
		}
		return nil
	})
}

func (b SecretServiceBackend) Delete(attrs map[string]string) error {
	return b.withSession(func(conn *dbus.Conn, session dbus.ObjectPath) error {
		_ = session
		svc := conn.Object(ssDest, ssPath)
		var unlocked, locked []dbus.ObjectPath
		if err := svc.Call(ssIface+".SearchItems", 0, attrs).Store(&unlocked, &locked); err != nil {
			return err
		}
		items := append(unlocked, locked...)
		if len(items) == 0 {
			return ErrNotFound
		}
		for _, p := range items {
			var prompt dbus.ObjectPath
			if err := conn.Object(ssDest, p).Call(ssItemIface+".Delete", 0).Store(&prompt); err != nil {
				return err
			}
		}
		return nil
	})
}
