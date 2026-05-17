package daemon

import (
	"errors"
	"fmt"
	"time"

	"github.com/ujjalsharma100/lockie/internal/store"
)

func (h *Handler) handleAliasAdd(req *Request, resp *Response) {
	var p AliasAddParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.Name == "" {
		setError(resp, ErrCodeInvalidParams, "name is required")
		return
	}
	if p.Value == "" {
		setError(resp, ErrCodeInvalidParams, "value is required")
		return
	}
	id, deduped, err := h.store.PutValue([]byte(p.Value))
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("store value: %v", err))
		return
	}
	if err := h.store.PutAlias(store.Alias{
		Project: p.Project,
		Name:    p.Name,
		ValueID: id,
	}); err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("store alias: %v", err))
		return
	}
	setResult(resp, AliasAddResult{ValueID: string(id), Deduped: deduped})
}

func (h *Handler) handleAliasList(req *Request, resp *Response) {
	var p AliasListParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	aliases, err := h.store.ListAliases(p.Project)
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("list aliases: %v", err))
		return
	}
	out := make([]AliasInfo, 0, len(aliases))
	for _, a := range aliases {
		out = append(out, aliasInfoFromStore(a))
	}
	setResult(resp, AliasListResult{Aliases: out})
}

func (h *Handler) handleAliasGet(req *Request, resp *Response) {
	var p AliasGetParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.Name == "" {
		setError(resp, ErrCodeInvalidParams, "name is required")
		return
	}
	a, err := h.store.GetAlias(p.Project, p.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			setError(resp, ErrCodeNotFound, fmt.Sprintf("alias %q not found", p.Name))
			return
		}
		setError(resp, ErrCodeInternal, fmt.Sprintf("get alias: %v", err))
		return
	}
	setResult(resp, aliasInfoFromStore(*a))
}

func (h *Handler) handleAliasForget(req *Request, resp *Response) {
	var p AliasForgetParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.Name == "" {
		setError(resp, ErrCodeInvalidParams, "name is required")
		return
	}
	a, err := h.store.GetAlias(p.Project, p.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			setError(resp, ErrCodeNotFound, fmt.Sprintf("alias %q not found", p.Name))
			return
		}
		setError(resp, ErrCodeInternal, fmt.Sprintf("get alias: %v", err))
		return
	}
	if err := h.store.DeleteAlias(p.Project, p.Name); err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("delete alias: %v", err))
		return
	}
	refs, err := h.store.ListAliasesByValue(a.ValueID)
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("list refs: %v", err))
		return
	}
	if len(refs) == 0 {
		if err := h.store.DeleteValue(a.ValueID); err != nil && !errors.Is(err, store.ErrNotFound) {
			setError(resp, ErrCodeInternal, fmt.Sprintf("gc value: %v", err))
			return
		}
	}
	setResult(resp, struct{}{})
}

func aliasInfoFromStore(a store.Alias) AliasInfo {
	return AliasInfo{
		Project:    a.Project,
		Name:       a.Name,
		ValueID:    string(a.ValueID),
		Hash:       a.Hash,
		CreatedAt:  a.CreatedAt.UTC().Format(time.RFC3339),
		LastUsedAt: a.LastUsedAt.UTC().Format(time.RFC3339),
	}
}
