package entities

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// World catalog files under content/reference: the npc, shop and object
// entities of the execution model (docs/10 section 7), generated from the
// wiki by internal/core/entities/catalogbuild. They are optional for Load
// (like location_maps.json) so that the generator can build a registry from
// the item catalog before the shop catalog, which names items, exists.
const (
	NPCsFile    = "npcs.json"
	ShopsFile   = "shops.json"
	ObjectsFile = "objects.json"
)

// NPCCatalog is content/reference/npcs.json.
type NPCCatalog struct {
	CatalogHeader
	NPCs []NPCEntry `json:"npcs"`
}

// NPCEntry is one wiki NPC page (an NPC or monster infobox). ID is the slug
// of the page title. GameIDs are the NPC ids of every version of the page
// (an NPC changes id with quest stages and appearance): what the executor
// looks for in game. Aliases are the names of other versions. LocationKeys
// are the wiki pages its infobox location links to; those that are catalog
// locations are its locations (Registry.LocationsOf). Monster marks a page
// with a monster infobox: it can be fought.
type NPCEntry struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WikiKey      WikiKey   `json:"wiki_key"`
	GameIDs      []int     `json:"game_ids"`
	Aliases      []string  `json:"aliases,omitempty"`
	LocationKeys []WikiKey `json:"location_keys,omitempty"`
	Monster      bool      `json:"monster,omitempty"`
}

// ShopCatalog is content/reference/shops.json.
type ShopCatalog struct {
	CatalogHeader
	Shops []ShopEntry `json:"shops"`
}

// ShopEntry is one wiki shop page. Owners are the npc ids its infobox owner
// links to (several when the same shop stands in several places).
// LocationKeys are, as for NPCs, the wiki pages its location links to.
// Stock lists the stock lines whose item and currency resolve to catalog
// items; Unresolved keeps the others with the reason, never guessed.
type ShopEntry struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	WikiKey      WikiKey          `json:"wiki_key"`
	Aliases      []string         `json:"aliases,omitempty"`
	Owners       []string         `json:"owners,omitempty"`
	LocationKeys []WikiKey        `json:"location_keys,omitempty"`
	Stock        []ShopStock      `json:"stock,omitempty"`
	Unresolved   []ShopUnresolved `json:"unresolved,omitempty"`
}

// ShopStock is one item a shop sells. Stock is the base stock the wiki
// lists; Infinite marks an unlimited stock; both absent means the wiki does
// not know. Price is what the shop charges, in Currency (an item id; 0 means
// coins, item 995).
type ShopStock struct {
	Item     int  `json:"item"`
	Stock    *int `json:"stock,omitempty"`
	Infinite bool `json:"infinite,omitempty"`
	Price    *int `json:"price,omitempty"`
	Currency int  `json:"currency,omitempty"`
}

// CoinsID is the item id of coins, the currency of a stock line without one.
const CoinsID = 995

// CurrencyID returns the item id the line is paid with.
func (s ShopStock) CurrencyID() int {
	if s.Currency == 0 {
		return CoinsID
	}
	return s.Currency
}

// ShopUnresolved is a stock line of the wiki that is not in Stock: the item
// name (or the currency) does not resolve to one catalog item.
type ShopUnresolved struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Reasons of unresolved stock lines.
const (
	StockItemNotInCatalog  = "item_not_in_catalog"
	StockItemAmbiguous     = "item_ambiguous"
	StockCurrencyNotAnItem = "currency_not_an_item"
)

// ObjectCatalog is content/reference/objects.json.
type ObjectCatalog struct {
	CatalogHeader
	Objects []ObjectEntry `json:"objects"`
}

// ObjectEntry is one wiki scenery page: a game object the executor
// interacts with. Name is the page title without its disambiguation
// ("Compost Bin (Port Phasmatys)" is named "Compost Bin"); GameIDs are the
// object ids of every version (open and closed, growth stages, ...).
type ObjectEntry struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	WikiKey WikiKey `json:"wiki_key"`
	GameIDs []int   `json:"game_ids"`
}

// worldEntry is what the name index needs from an npc, shop or object.
type worldEntry struct {
	name    string
	aliases []string
	wikiKey WikiKey
}

// worldIndex is the catalog of one world kind.
type worldIndex struct {
	kind  Kind
	file  string
	names nameIndex
	keys  map[string]WikiKey
	wiki  map[WikiKey]string
}

func newWorldIndex(kind Kind, file string) *worldIndex {
	return &worldIndex{kind: kind, file: file, names: nameIndex{}, keys: map[string]WikiKey{}, wiki: map[WikiKey]string{}}
}

// add indexes one entry and reports a problem with it, or "".
func (ix *worldIndex) add(id string, e worldEntry) string {
	ref := Ref{Kind: ix.kind, ID: id}
	if err := ref.Validate(); err != nil {
		return err.Error()
	}
	if _, dup := ix.keys[id]; dup {
		return fmt.Sprintf("duplicate %s id %q", ix.kind, id)
	}
	if e.name == "" || e.wikiKey == "" {
		return fmt.Sprintf("%s %q: missing name or wiki_key", ix.kind, id)
	}
	key := CanonicalWikiKey(string(e.wikiKey))
	if prev, dup := ix.wiki[key]; dup {
		return fmt.Sprintf("%s %q and %q share wiki page %q", ix.kind, prev, id, key)
	}
	ix.keys[id] = key
	ix.wiki[key] = id
	ix.names.add(e.name, id, MatchName)
	for _, a := range e.aliases {
		ix.names.add(a, id, MatchAlias)
	}
	return ""
}

// resolve looks a name up. Several pages may carry one name (35 "Banker"
// pages); a bare name then means the page titled with it, the others
// ("Banker (Grand Exchange)") resolve by their own title, which is an alias
// of no one but their page key. A name no page is titled with stays
// ambiguous.
func (ix *worldIndex) resolve(name string) Resolution {
	k := NormalizeName(name)
	hits := ix.names[k]
	if len(hits) > 1 {
		var own []nameHit
		for _, h := range hits {
			if h.match == MatchName && NormalizeName(string(ix.keys[h.id])) == k {
				own = append(own, h)
			}
		}
		if len(own) == 1 {
			hits = own
		}
	}
	if res, ok := fromHits(ix.kind, hits); ok {
		return res
	}
	// The page title itself ("Banker (Grand Exchange)", "Cook (Lumbridge)").
	if id, ok := ix.wiki[CanonicalWikiKey(name)]; ok {
		return resolved(Ref{Kind: ix.kind, ID: id}, MatchWikiKey)
	}
	return unresolved(ReasonNotInCatalog)
}

// readWorld decodes an optional world catalog; a missing file is an empty
// catalog.
func readWorld(fsys fs.FS, name string, into any) error {
	data, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// loadWorld reads the npc, shop and object catalogs into r.
func (r *Registry) loadWorld(fsys fs.FS, fail func(string, ...any)) {
	var (
		nc NPCCatalog
		sc ShopCatalog
		oc ObjectCatalog
	)
	for name, into := range map[string]any{NPCsFile: &nc, ShopsFile: &sc, ObjectsFile: &oc} {
		if err := readWorld(fsys, name, into); err != nil {
			fail("%v", err)
		}
	}
	r.Versions[KindNPC], r.Versions[KindShop], r.Versions[KindObject] = nc.Version, sc.Version, oc.Version
	r.npcIx, r.shopIx, r.objectIx = newWorldIndex(KindNPC, NPCsFile), newWorldIndex(KindShop, ShopsFile), newWorldIndex(KindObject, ObjectsFile)
	r.npcs, r.shops, r.objects = map[string]*NPCEntry{}, map[string]*ShopEntry{}, map[string]*ObjectEntry{}
	r.npcByGame, r.objectByGame, r.shopsByOwner = map[int][]string{}, map[int][]string{}, map[string][]string{}
	for i := range nc.NPCs {
		n := &nc.NPCs[i]
		if msg := r.npcIx.add(n.ID, worldEntry{n.Name, n.Aliases, n.WikiKey}); msg != "" {
			fail("%s: %s", NPCsFile, msg)
			continue
		}
		if len(n.GameIDs) == 0 {
			fail("%s: npc %q has no game ids", NPCsFile, n.ID)
		}
		r.npcs[n.ID] = n
		for _, g := range n.GameIDs {
			r.npcByGame[g] = appendOnce(r.npcByGame[g], n.ID)
		}
	}
	for i := range oc.Objects {
		o := &oc.Objects[i]
		if msg := r.objectIx.add(o.ID, worldEntry{o.Name, nil, o.WikiKey}); msg != "" {
			fail("%s: %s", ObjectsFile, msg)
			continue
		}
		if len(o.GameIDs) == 0 {
			fail("%s: object %q has no game ids", ObjectsFile, o.ID)
		}
		r.objects[o.ID] = o
		for _, g := range o.GameIDs {
			r.objectByGame[g] = appendOnce(r.objectByGame[g], o.ID)
		}
	}
	for i := range sc.Shops {
		s := &sc.Shops[i]
		if msg := r.shopIx.add(s.ID, worldEntry{s.Name, s.Aliases, s.WikiKey}); msg != "" {
			fail("%s: %s", ShopsFile, msg)
			continue
		}
		for _, o := range s.Owners {
			if _, ok := r.npcs[o]; !ok {
				fail("%s: shop %q: unknown owner npc %q", ShopsFile, s.ID, o)
			}
			r.shopsByOwner[o] = appendOnce(r.shopsByOwner[o], s.ID)
		}
		for _, st := range s.Stock {
			for _, id := range []int{st.Item, st.CurrencyID()} {
				if _, ok := r.Item(id); !ok {
					fail("%s: shop %q: unknown item %d", ShopsFile, s.ID, id)
				}
			}
			if st.Stock != nil && (*st.Stock < 0 || st.Infinite) {
				fail("%s: shop %q: item %d: stock is a count or infinite", ShopsFile, s.ID, st.Item)
			}
		}
		r.shops[s.ID] = s
	}
}

// NPC returns an npc entry by id.
func (r *Registry) NPC(id string) (*NPCEntry, bool) {
	n, ok := r.npcs[id]
	return n, ok
}

// Shop returns a shop entry by id.
func (r *Registry) Shop(id string) (*ShopEntry, bool) {
	s, ok := r.shops[id]
	return s, ok
}

// Object returns an object entry by id.
func (r *Registry) Object(id string) (*ObjectEntry, bool) {
	o, ok := r.objects[id]
	return o, ok
}

// ShopStockOf returns the stock line of item in a shop: the exact id, or
// its unnoted form when item is a noted id (shops sell unnoted items).
func (r *Registry) ShopStockOf(shopID string, item int) (ShopStock, bool) {
	s, ok := r.shops[shopID]
	if !ok {
		return ShopStock{}, false
	}
	base := item
	if f, ok := r.Item(item); ok {
		base = f.BaseID
	}
	for _, st := range s.Stock {
		if st.Item == base {
			return st, true
		}
	}
	return ShopStock{}, false
}

// ResolveNPC resolves an NPC by name, version name or page title. A name
// several pages carry resolves to the page titled with it, when there is
// one, and is ambiguous otherwise ("Cook": Lumbridge, Rogues' Den, ...).
func (r *Registry) ResolveNPC(name string) Resolution { return r.npcIx.resolve(name) }

// ResolveShop resolves a shop by name or page title.
func (r *Registry) ResolveShop(name string) Resolution { return r.shopIx.resolve(name) }

// ResolveObject resolves a game object by name or page title.
func (r *Registry) ResolveObject(name string) Resolution { return r.objectIx.resolve(name) }

// LocationsOf returns the catalog locations an npc or shop is at: the
// location pages its wiki infobox links to, in that order, plus for a shop
// those of its owners. Pages that are not catalog locations are skipped.
func (r *Registry) LocationsOf(ref Ref) []Ref {
	var keys []WikiKey
	switch ref.Kind {
	case KindNPC:
		if n, ok := r.npcs[ref.ID]; ok {
			keys = n.LocationKeys
		}
	case KindShop:
		if s, ok := r.shops[ref.ID]; ok {
			keys = append(keys, s.LocationKeys...)
			for _, o := range s.Owners {
				keys = append(keys, r.npcs[o].LocationKeys...)
			}
		}
	}
	var out []Ref
	seen := map[Ref]bool{}
	for _, k := range keys {
		for _, loc := range r.byWiki[CanonicalWikiKey(string(k))] {
			if loc.Kind == KindLocation && !seen[loc] {
				seen[loc] = true
				out = append(out, loc)
			}
		}
	}
	return out
}

// NPCsByGameID returns the npc pages whose versions include the game NPC id,
// in id order: one page is the npc; several are ambiguous (never guessed);
// none is an id outside the catalog.
func (r *Registry) NPCsByGameID(id int) []Ref { return refsOf(KindNPC, r.npcByGame[id]) }

// ObjectsByGameID returns the object pages whose versions include the game
// object id, like NPCsByGameID.
func (r *Registry) ObjectsByGameID(id int) []Ref { return refsOf(KindObject, r.objectByGame[id]) }

// ShopsOwnedBy returns the shops whose wiki infobox names the npc as owner.
func (r *Registry) ShopsOwnedBy(npcID string) []Ref { return refsOf(KindShop, r.shopsByOwner[npcID]) }

// LocationsAt returns the catalog locations whose area (docs/10 section 7)
// contains the tile, in id order: an area may lie inside another ("Lumbridge
// Castle" in "Lumbridge"), so more than one is not an error.
func (r *Registry) LocationsAt(x, y, plane int) []Ref {
	var ids []string
	for id, m := range r.locationMaps {
		if m.Area != nil && m.Area.Contains(x, y, plane) {
			ids = append(ids, id)
		}
	}
	return refsOf(KindLocation, ids)
}

func refsOf(kind Kind, ids []string) []Ref {
	out := make([]Ref, 0, len(ids))
	for _, id := range ids {
		out = append(out, Ref{Kind: kind, ID: id})
	}
	slices.SortFunc(out, func(a, b Ref) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func appendOnce(list []string, id string) []string {
	if slices.Contains(list, id) {
		return list
	}
	return append(list, id)
}

// WorldCounts returns how many npcs, shops and objects the catalogs hold.
func (r *Registry) WorldCounts() map[Kind]int {
	return map[Kind]int{KindNPC: len(r.npcs), KindShop: len(r.shops), KindObject: len(r.objects)}
}

// worldKnown reports whether a world ref names a catalog entry.
func (r *Registry) worldKnown(ref Ref) bool {
	switch ref.Kind {
	case KindNPC:
		_, ok := r.npcs[ref.ID]
		return ok
	case KindShop:
		_, ok := r.shops[ref.ID]
		return ok
	case KindObject:
		_, ok := r.objects[ref.ID]
		return ok
	}
	return false
}

// worldDisplay returns the display name and wiki key of a world ref.
func (r *Registry) worldDisplay(ref Ref) (string, WikiKey, bool) {
	switch ref.Kind {
	case KindNPC:
		if n, ok := r.npcs[ref.ID]; ok {
			return n.Name, n.WikiKey, true
		}
	case KindShop:
		if s, ok := r.shops[ref.ID]; ok {
			return s.Name, s.WikiKey, true
		}
	case KindObject:
		if o, ok := r.objects[ref.ID]; ok {
			return o.Name, o.WikiKey, true
		}
	}
	return "", "", false
}
