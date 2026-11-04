package ingest

import (
	"fmt"
	"strconv"
	"strings"
)

type validationErrors []string

func (e validationErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	return fmt.Errorf("invalid ingest contract: %s", strings.Join(e, "; "))
}

func (m Manifest) Validate() error {
	var errs validationErrors
	if m.ContractVersion != ContractVersion {
		errs = append(errs, "contract_version must be 1")
	}
	if m.ManifestVersion < 1 {
		errs = append(errs, "manifest_version must be positive")
	}
	if m.IntervalSeconds < 5 {
		errs = append(errs, "interval_seconds must be at least 5")
	}
	if m.Varbits == nil {
		errs = append(errs, "varbits must be a JSON array (use [] when empty)")
	}
	if m.Varps == nil {
		errs = append(errs, "varps must be a JSON array (use [] when empty)")
	}
	if m.Collections == nil {
		errs = append(errs, "collections must be a JSON array (use [] when empty)")
	}

	scopes := make(map[TokenScope]bool, len(m.TokenScopes))
	for i, scope := range m.TokenScopes {
		switch scope {
		case ScopeStateWrite, ScopeItemsBank, ScopeItemsInventory, ScopeItemsEquipment:
		default:
			errs = append(errs, fmt.Sprintf("token_scopes[%d] is unknown", i))
		}
		if scopes[scope] {
			errs = append(errs, fmt.Sprintf("token_scopes contains duplicate %q", scope))
		}
		scopes[scope] = true
	}
	if !scopes[ScopeStateWrite] {
		errs = append(errs, "token_scopes must contain state:write")
	}
	validateScopeFlag := func(path string, enabled bool, scope TokenScope) {
		if enabled != scopes[scope] {
			errs = append(errs, fmt.Sprintf("%s must match token scope %s", path, scope))
		}
	}
	validateScopeFlag("items.bank", m.Items.Bank, ScopeItemsBank)
	validateScopeFlag("items.inventory", m.Items.Inventory, ScopeItemsInventory)
	validateScopeFlag("items.equipment", m.Items.Equipment, ScopeItemsEquipment)

	validateNonNegativeUniqueIDs(&errs, "varbits", m.Varbits)
	validateNonNegativeUniqueIDs(&errs, "varps", m.Varps)
	validatePositiveUniqueIDs(&errs, "collections", m.Collections)
	validatePositiveUniqueIDs(&errs, "items.only_item_ids", m.Items.OnlyItemIDs)
	return errs.err()
}

func (p Push) Validate() error {
	var errs validationErrors
	if p.ContractVersion != ContractVersion {
		errs = append(errs, "contract_version must be 1")
	}
	if strings.TrimSpace(p.Username) == "" {
		errs = append(errs, "username is required")
	}
	switch p.Profile {
	case "STANDARD", "BETA", "DEPRECATED", "OTHER":
	default:
		errs = append(errs, "profile is invalid")
	}
	if p.AccountType != "" {
		switch p.AccountType {
		case "NORMAL", "IRONMAN", "HARDCORE_IRONMAN", "ULTIMATE_IRONMAN", "GROUP_IRONMAN", "HARDCORE_GROUP_IRONMAN", "UNRANKED_GROUP_IRONMAN":
		default:
			errs = append(errs, "account_type is invalid")
		}
	}
	if p.ManifestVersion < 1 {
		errs = append(errs, "manifest_version must be positive")
	}
	if p.ClientSeq < 1 {
		errs = append(errs, "client_seq must be positive")
	}
	if p.SentAt.IsZero() {
		errs = append(errs, "sent_at is required")
	}
	if p.Data.CollectionLogItemCount != nil && *p.Data.CollectionLogItemCount < 0 {
		errs = append(errs, "data.collectionLogItemCount must not be negative")
	}
	for _, field := range []struct {
		path   string
		values *IntMap
	}{
		{path: "data.varb", values: p.Data.Varbits},
		{path: "data.varp", values: p.Data.Varps},
		{path: "data.level", values: p.Data.Levels},
		{path: "data.xp", values: p.Data.XP},
		{path: "data.boosted", values: p.Data.Boosted},
	} {
		if field.values != nil && *field.values == nil {
			errs = append(errs, field.path+" must be a JSON object (use {} when empty)")
		}
	}

	switch p.Kind {
	case PushFull:
		p.validateFull(&errs)
	case PushDelta:
		p.validateDelta(&errs)
	default:
		errs = append(errs, "kind must be full or delta")
	}
	return errs.err()
}

func (p Push) validateFull(errs *validationErrors) {
	requiredMap := func(path string, values *IntMap) {
		if values == nil {
			*errs = append(*errs, path+" is required for a full push")
		}
	}
	requiredMap("data.varb", p.Data.Varbits)
	requiredMap("data.varp", p.Data.Varps)
	requiredMap("data.level", p.Data.Levels)
	requiredMap("data.xp", p.Data.XP)
	requiredMap("data.boosted", p.Data.Boosted)
	if p.Data.Items == nil {
		*errs = append(*errs, "data.items is required for a full push")
		return
	}
	p.validateObservation(errs, "data.items.bank", p.Data.Items.Bank, true)
	p.validateObservation(errs, "data.items.inventory", p.Data.Items.Inventory, true)
	p.validateObservation(errs, "data.items.equipment", p.Data.Items.Equipment, true)
}

func (p Push) validateDelta(errs *validationErrors) {
	if p.Data.Items == nil {
		return
	}
	p.validateObservation(errs, "data.items.bank", p.Data.Items.Bank, false)
	p.validateObservation(errs, "data.items.inventory", p.Data.Items.Inventory, false)
	p.validateObservation(errs, "data.items.equipment", p.Data.Items.Equipment, false)
}

func (p Push) validateObservation(errs *validationErrors, path string, obs *ItemObservation, full bool) {
	if obs == nil {
		if full {
			*errs = append(*errs, path+" is required for a full push")
		}
		return
	}
	if obs.ObservedAt.IsZero() {
		*errs = append(*errs, path+".observed_at is required")
	} else if !p.SentAt.IsZero() && obs.ObservedAt.After(p.SentAt) {
		*errs = append(*errs, path+".observed_at must not be after sent_at")
	}

	switch obs.State {
	case ItemUnavailable:
		if obs.Revision != nil {
			*errs = append(*errs, path+".revision must be absent when state is unavailable")
		}
		if obs.LiveEpoch != "" {
			*errs = append(*errs, path+".live_epoch must be absent when state is unavailable")
		}
		if obs.Quantities != nil {
			*errs = append(*errs, path+".quantities must be absent when state is unavailable")
		}
		return
	case ItemRestored:
		if !full {
			*errs = append(*errs, path+" with state restored is valid only in a full push")
		}
		if obs.LiveEpoch != "" {
			*errs = append(*errs, path+".live_epoch must be absent when state is restored")
		}
	case ItemLive:
		if strings.TrimSpace(obs.LiveEpoch) == "" {
			*errs = append(*errs, path+".live_epoch is required when state is live")
		}
	default:
		*errs = append(*errs, path+".state is invalid")
		return
	}

	if obs.Revision == nil {
		*errs = append(*errs, path+".revision is required when state is "+string(obs.State))
	} else if *obs.Revision < 0 {
		*errs = append(*errs, path+".revision must not be negative")
	}
	if obs.Quantities == nil || *obs.Quantities == nil {
		*errs = append(*errs, path+".quantities is required when state is "+string(obs.State))
		return
	}
	for itemID, qty := range *obs.Quantities {
		if !canonicalPositiveID(itemID) {
			*errs = append(*errs, fmt.Sprintf("%s.quantities key %q must be a positive canonical item id", path, itemID))
		}
		if qty < 0 {
			*errs = append(*errs, fmt.Sprintf("%s.quantities[%q] must not be negative", path, itemID))
		} else if full && qty == 0 {
			*errs = append(*errs, fmt.Sprintf("%s.quantities[%q] must be positive in a full push", path, itemID))
		}
	}
}

// ValidateAgainst checks the push by itself and then enforces the effective
// item permissions advertised by a manifest.
func (p Push) ValidateAgainst(m Manifest) error {
	var errs validationErrors
	if err := m.Validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := p.Validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if p.ContractVersion != m.ContractVersion {
		errs = append(errs, "push and manifest contract_version differ")
	}
	if p.ManifestVersion != m.ManifestVersion {
		errs = append(errs, "push and manifest manifest_version differ")
	}
	if p.Data.Items == nil {
		return errs.err()
	}
	validateAllowed := func(path string, allowed bool, obs *ItemObservation) {
		if obs != nil && obs.State != ItemUnavailable && !allowed {
			errs = append(errs, path+" is not enabled by the manifest/token scope")
		}
		if obs == nil || obs.Quantities == nil || len(m.Items.OnlyItemIDs) == 0 {
			return
		}
		allowedIDs := make(map[string]bool, len(m.Items.OnlyItemIDs))
		for _, id := range m.Items.OnlyItemIDs {
			allowedIDs[strconv.FormatInt(id, 10)] = true
		}
		for id := range *obs.Quantities {
			if !allowedIDs[id] {
				errs = append(errs, fmt.Sprintf("%s.quantities[%q] is outside items.only_item_ids", path, id))
			}
		}
	}
	validateAllowed("data.items.bank", m.Items.Bank, p.Data.Items.Bank)
	validateAllowed("data.items.inventory", m.Items.Inventory, p.Data.Items.Inventory)
	validateAllowed("data.items.equipment", m.Items.Equipment, p.Data.Items.Equipment)
	return errs.err()
}

func (r Response) Validate() error {
	var errs validationErrors
	if r.ContractVersion != ContractVersion {
		errs = append(errs, "contract_version must be 1")
	}
	if r.AcceptedSeq < 0 {
		errs = append(errs, "accepted_seq must not be negative")
	}
	if r.ManifestVersion < 1 {
		errs = append(errs, "manifest_version must be positive")
	}
	if r.StepsChanged != nil && *r.StepsChanged < 0 {
		errs = append(errs, "steps_changed must not be negative")
	}
	return errs.err()
}

func validatePositiveUniqueIDs(errs *validationErrors, path string, ids []int64) {
	seen := make(map[int64]bool, len(ids))
	for i, id := range ids {
		if id < 1 {
			*errs = append(*errs, fmt.Sprintf("%s[%d] must be positive", path, i))
		}
		if seen[id] {
			*errs = append(*errs, fmt.Sprintf("%s contains duplicate %d", path, id))
		}
		seen[id] = true
	}
}

func validateNonNegativeUniqueIDs(errs *validationErrors, path string, ids []int64) {
	seen := make(map[int64]bool, len(ids))
	for i, id := range ids {
		if id < 0 {
			*errs = append(*errs, fmt.Sprintf("%s[%d] must not be negative", path, i))
		}
		if seen[id] {
			*errs = append(*errs, fmt.Sprintf("%s contains duplicate %d", path, id))
		}
		seen[id] = true
	}
}

func canonicalPositiveID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}
