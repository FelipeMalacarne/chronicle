package catalogbuild

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// Inputs of the world catalogs (docs/10 section 7), written by
// tools/extract_wiki_world.py and tools/fetch_wiki_locations.py.
const (
	RawNPCs         = "data/wiki/npcs.json"
	RawShops        = "data/wiki/shops.json"
	RawObjects      = "data/wiki/objects.json"
	RawLocationMaps = "data/wiki/location_maps.json"
	// RawWorldIDs holds the curated id overrides of world pages.
	RawWorldIDs = "data/wiki/world_ids.json"
)

// worldIDs is data/wiki/world_ids.json.
type worldIDs struct {
	NPCIDs    []idOverride `json:"npc_ids"`
	ObjectIDs []idOverride `json:"object_ids"`
}

type rawWorldSource struct {
	Wiki      string `json:"wiki"`
	FetchedAt string `json:"fetched_at"`
}

// rawNPCs is data/wiki/npcs.json: one row per version of an NPC infobox
// (npcs) or monster infobox (monsters).
type rawNPCs struct {
	Source   rawWorldSource `json:"source"`
	NPCs     []rawNPCRow    `json:"npcs"`
	Monsters []rawNPCRow    `json:"monsters"`
}

type rawNPCRow struct {
	Page     string `json:"page"`
	Version  string `json:"version"`
	Name     string `json:"name"`
	IDs      []int  `json:"ids"`
	Location string `json:"location"`
}

// rawShops is data/wiki/shops.json: shop infobox rows and stock lines.
type rawShops struct {
	Source rawWorldSource `json:"source"`
	Shops  []struct {
		Page     string `json:"page"`
		Version  string `json:"version"`
		Name     string `json:"name"`
		Owner    string `json:"owner"`
		Location string `json:"location"`
	} `json:"shops"`
	Stock []struct {
		SoldBy   string `json:"sold_by"`
		Item     string `json:"item"`
		Price    string `json:"price"`
		Stock    string `json:"stock"`
		Currency string `json:"currency"`
	} `json:"stock"`
}

// rawObjects is data/wiki/objects.json: scenery infobox rows.
type rawObjects struct {
	Source  rawWorldSource `json:"source"`
	Objects []struct {
		Page    string `json:"page"`
		Version string `json:"version"`
		IDs     []int  `json:"ids"`
	} `json:"objects"`
}

// rawLocationMaps is data/wiki/location_maps.json: the infobox map template
// of each catalog location's page.
type rawLocationMaps struct {
	Locations []struct {
		ID     string `json:"id"`
		Page   string `json:"page"`
		RevID  int    `json:"revid"`
		Map    string `json:"map"`
		Reason string `json:"reason"`
	} `json:"locations"`
}

// idOverride gives a wiki page an id other than the slug of its title (two
// titles that slug alike, a title without letters).
type idOverride struct {
	Page string `json:"page"`
	ID   string `json:"id"`
	Note string `json:"note"`
}

var (
	qualifierRe = regexp.MustCompile(`^(.*?)\s*\(([^()]*)\)$`)
	// junkQualifier marks pages of content that is not in the game: holiday
	// events, leagues, Deadman, betas, historical and removed versions (the
	// same exclusions as the item extract). "random event" NPCs stay.
	junkQualifier = regexp.MustCompile(`(?i)\b(?:beta|removed|deadman|leagues?|last man standing|april fools|events?)\b|(?:^|, )historical$`)
	linkRe        = regexp.MustCompile(`\[\[([^\]|#]+)`)
	markupRe      = regexp.MustCompile(`<|\[\[|\{\{`)
)

// junkPage reports whether a page title is for content outside the game.
func junkPage(title string) bool {
	m := qualifierRe.FindStringSubmatch(title)
	if m == nil {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(m[2]))
	return q != "random event" && junkQualifier.MatchString(q)
}

// baseTitle drops the disambiguation of a title: "Cook (Lumbridge)" → "Cook".
func baseTitle(title string) string {
	if m := qualifierRe.FindStringSubmatch(title); len(m) == 3 && m[1] != "" {
		return m[1]
	}
	return title
}

// cleanName is an in-game name without wiki markup; "" for unnamed NPCs
// (the wiki writes "null").
func cleanName(s string) string {
	if loc := markupRe.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "null") {
		return ""
	}
	return s
}

// links returns the canonical keys of the article links in wikitext, in
// order, without repeats; files, categories and other namespaces are skipped.
func links(wikitext string, into []entities.WikiKey) []entities.WikiKey {
	for _, m := range linkRe.FindAllStringSubmatch(wikitext, -1) {
		t := strings.TrimSpace(m[1])
		if t == "" || strings.Contains(t, ":") {
			continue
		}
		k := entities.CanonicalWikiKey(t)
		if !slices.Contains(into, k) {
			into = append(into, k)
		}
	}
	return into
}

// pageIDs assigns the id of each page: its slug, or the curated override.
// Two pages with one id fail the build, so a new collision after a refresh
// is decided by a person (an override in world_ids.json), never by
// renaming an id something already references.
func pageIDs(kind string, pages []string, overrides []idOverride) (map[string]string, error) {
	over := map[string]string{}
	for _, o := range overrides {
		if err := (entities.Ref{Kind: entities.Kind(kind), ID: o.ID}).Validate(); err != nil {
			return nil, fmt.Errorf("catalogbuild: %s id override %q: %w", kind, o.Page, err)
		}
		over[o.Page] = o.ID
	}
	ids := map[string]string{}
	byID := map[string]string{}
	for _, p := range pages {
		id, ok := over[p]
		if !ok {
			id = entities.Slug(p)
		}
		if id == "" {
			return nil, fmt.Errorf("catalogbuild: %s page %q has no slug: add a %s_ids override in %s", kind, p, kind, RawWorldIDs)
		}
		if prev, dup := byID[id]; dup {
			return nil, fmt.Errorf("catalogbuild: %s pages %q and %q share id %q: add a %s_ids override in %s", kind, prev, p, id, kind, RawWorldIDs)
		}
		byID[id] = p
		ids[p] = id
	}
	for p := range over {
		if _, used := ids[p]; !used {
			return nil, fmt.Errorf("catalogbuild: %s id override for unknown page %q", kind, p)
		}
	}
	return ids, nil
}

// pickName chooses the entity name among the names of a page's versions:
// the one spelled like the page title without its disambiguation, else the
// first. The others become aliases.
func pickName(title string, names []string) (string, []string) {
	var distinct []string
	for _, n := range names {
		if n != "" && !slices.Contains(distinct, n) {
			distinct = append(distinct, n)
		}
	}
	if len(distinct) == 0 {
		return "", nil
	}
	main := distinct[0]
	base := entities.NormalizeName(baseTitle(title))
	for _, n := range distinct {
		if entities.NormalizeName(n) == base {
			main = n
			break
		}
	}
	var aliases []string
	for _, n := range distinct {
		if n != main && entities.NormalizeName(n) != entities.NormalizeName(main) {
			aliases = append(aliases, n)
		}
	}
	sort.Strings(aliases)
	return main, aliases
}

// worldStats counts what the world builders dropped, for the catalog notes.
type worldStats struct {
	junk, noIDs, unnamed int
}

// buildNPCs groups the NPC and monster infobox rows by page. A page of
// content outside the game (events, leagues, betas, historical), a page
// without NPC ids and a page whose every version is unnamed are left out.
func buildNPCs(raw rawNPCs, overrides []idOverride) (*entities.NPCCatalog, worldStats, error) {
	type page struct {
		rows    []rawNPCRow
		monster bool
	}
	pages := map[string]*page{}
	add := func(rows []rawNPCRow, monster bool) {
		for _, r := range rows {
			p := pages[r.Page]
			if p == nil {
				p = &page{}
				pages[r.Page] = p
			}
			p.rows = append(p.rows, r)
			p.monster = p.monster || monster
		}
	}
	add(raw.NPCs, false)
	add(raw.Monsters, true)
	var st worldStats
	var kept []string
	for title, p := range pages {
		var ids []int
		var names []string
		for _, r := range p.rows {
			ids = append(ids, r.IDs...)
			names = append(names, cleanName(r.Name))
		}
		switch name, _ := pickName(title, names); {
		case junkPage(title):
			st.junk++
		case len(ids) == 0:
			st.noIDs++
		case name == "":
			st.unnamed++
		default:
			kept = append(kept, title)
		}
	}
	sort.Strings(kept)
	ids, err := pageIDs("npc", kept, overrides)
	if err != nil {
		return nil, st, err
	}
	nc := &entities.NPCCatalog{}
	for _, title := range kept {
		p := pages[title]
		sort.SliceStable(p.rows, func(i, j int) bool { return p.rows[i].Version < p.rows[j].Version })
		e := entities.NPCEntry{ID: ids[title], WikiKey: entities.CanonicalWikiKey(title), Monster: p.monster}
		var names []string
		for _, r := range p.rows {
			names = append(names, cleanName(r.Name))
			e.GameIDs = append(e.GameIDs, r.IDs...)
			e.LocationKeys = links(r.Location, e.LocationKeys)
		}
		e.Name, e.Aliases = pickName(title, names)
		slices.Sort(e.GameIDs)
		e.GameIDs = slices.Compact(e.GameIDs)
		nc.NPCs = append(nc.NPCs, e)
	}
	sort.Slice(nc.NPCs, func(i, j int) bool { return nc.NPCs[i].ID < nc.NPCs[j].ID })
	return nc, st, nil
}

// buildObjects groups the scenery infobox rows by page; the same pages as
// for NPCs are left out. The name is the page title without disambiguation.
func buildObjects(raw rawObjects, overrides []idOverride) (*entities.ObjectCatalog, worldStats, error) {
	byPage := map[string][]int{}
	for _, r := range raw.Objects {
		byPage[r.Page] = append(byPage[r.Page], r.IDs...)
	}
	var st worldStats
	var kept []string
	for title, ids := range byPage {
		switch {
		case junkPage(title):
			st.junk++
		case len(ids) == 0:
			st.noIDs++
		default:
			kept = append(kept, title)
		}
	}
	sort.Strings(kept)
	ids, err := pageIDs("object", kept, overrides)
	if err != nil {
		return nil, st, err
	}
	oc := &entities.ObjectCatalog{}
	for _, title := range kept {
		gameIDs := slices.Clone(byPage[title])
		slices.Sort(gameIDs)
		oc.Objects = append(oc.Objects, entities.ObjectEntry{
			ID: ids[title], Name: baseTitle(title), WikiKey: entities.CanonicalWikiKey(title), GameIDs: slices.Compact(gameIDs),
		})
	}
	sort.Slice(oc.Objects, func(i, j int) bool { return oc.Objects[i].ID < oc.Objects[j].ID })
	return oc, st, nil
}

// itemResolver resolves a wiki item name to one catalog id, the way a guide
// name resolves (entities.Registry.ResolveItem).
type itemResolver func(name string) (int, string)

// buildShops groups the shop infobox rows by page (one shop standing in
// several places has a version per place) and attaches the stock lines sold
// by the page. Owners are the owner links that are catalog NPCs; stock items
// and currencies resolve through the item catalog or go to unresolved.
func buildShops(raw rawShops, npcs *entities.NPCCatalog, resolveItem itemResolver) (*entities.ShopCatalog, int, error) {
	npcByKey := map[entities.WikiKey]string{}
	for _, n := range npcs.NPCs {
		npcByKey[n.WikiKey] = n.ID
	}
	type page struct {
		names, owners []string
		locs          []entities.WikiKey
	}
	pages := map[string]*page{}
	var titles []string
	for _, r := range raw.Shops {
		p := pages[r.Page]
		if p == nil {
			p = &page{}
			pages[r.Page] = p
			titles = append(titles, r.Page)
		}
		p.names = append(p.names, cleanName(r.Name))
		for _, k := range links(r.Owner, nil) {
			if id, ok := npcByKey[k]; ok && !slices.Contains(p.owners, id) {
				p.owners = append(p.owners, id)
			}
		}
		p.locs = links(r.Location, p.locs)
	}
	sort.Strings(titles)
	ids, err := pageIDs("shop", titles, nil)
	if err != nil {
		return nil, 0, err
	}
	byKey := map[entities.WikiKey]*entities.ShopEntry{}
	sc := &entities.ShopCatalog{}
	for _, title := range titles {
		p := pages[title]
		name, aliases := pickName(title, p.names)
		if name == "" {
			name = baseTitle(title)
		}
		sc.Shops = append(sc.Shops, entities.ShopEntry{
			ID: ids[title], Name: name, WikiKey: entities.CanonicalWikiKey(title), Aliases: aliases, Owners: p.owners, LocationKeys: p.locs,
		})
	}
	for i := range sc.Shops {
		byKey[sc.Shops[i].WikiKey] = &sc.Shops[i]
	}
	elsewhere := 0
	for _, line := range raw.Stock {
		s, ok := byKey[entities.CanonicalWikiKey(line.SoldBy)]
		if !ok {
			elsewhere++ // sold by an NPC or a page without a shop infobox
			continue
		}
		item, reason := resolveItem(line.Item)
		currency := 0
		if reason == "" && line.Currency != "" && !strings.EqualFold(line.Currency, "Coins") {
			if currency, reason = resolveItem(line.Currency); reason != "" {
				reason = entities.StockCurrencyNotAnItem
			}
		}
		if reason != "" {
			u := entities.ShopUnresolved{Name: line.Item, Reason: reason}
			if reason == entities.StockCurrencyNotAnItem {
				u.Name = line.Item + " (" + line.Currency + ")"
			}
			if !slices.Contains(s.Unresolved, u) {
				s.Unresolved = append(s.Unresolved, u)
			}
			continue
		}
		if currency == entities.CoinsID {
			currency = 0
		}
		stock := entities.ShopStock{Item: item, Currency: currency, Price: count(line.Price)}
		switch v := strings.TrimSpace(line.Stock); {
		case strings.HasPrefix(v, "∞"):
			stock.Infinite = true
		default:
			stock.Stock = count(v)
		}
		if !slices.ContainsFunc(s.Stock, func(x entities.ShopStock) bool { return sameStock(x, stock) }) {
			s.Stock = append(s.Stock, stock)
		}
	}
	for i := range sc.Shops {
		s := &sc.Shops[i]
		sort.SliceStable(s.Stock, func(a, b int) bool { return s.Stock[a].Item < s.Stock[b].Item })
		sort.Slice(s.Unresolved, func(a, b int) bool { return s.Unresolved[a].Name < s.Unresolved[b].Name })
	}
	sort.Slice(sc.Shops, func(i, j int) bool { return sc.Shops[i].ID < sc.Shops[j].ID })
	return sc, elsewhere, nil
}

func sameStock(a, b entities.ShopStock) bool {
	eq := func(x, y *int) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	return a.Item == b.Item && a.Currency == b.Currency && a.Infinite == b.Infinite && eq(a.Stock, b.Stock) && eq(a.Price, b.Price)
}

// count parses a non-negative wiki number ("1,000"); nil when it is not one
// ("N/A", "Unknown").
func count(s string) *int {
	n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

var pairRe = regexp.MustCompile(`^\s*(\d{3,5})\s*,\s*(\d{3,5})\s*$`)

// mapPoint reads a wiki {{Map}} template: the point (the center of its
// points, rounded half to even), its plane and the area its shape gives;
// reason says why there is no point (an instance map, no coordinates).
func mapPoint(tpl string) (x, y, plane int, area *entities.Area, reason string) {
	body := strings.TrimSpace(strings.Trim(tpl, "{}"))
	if len(body) < 3 {
		return 0, 0, 0, nil, "map without a point"
	}
	args := strings.Split(body[3:], "|")
	named := map[string]string{}
	var shape [][2]int
	for _, a := range args {
		a = strings.TrimSpace(a)
		if k, v, ok := strings.Cut(a, "="); ok {
			named[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		} else if m := pairRe.FindStringSubmatch(a); m != nil {
			px, _ := strconv.Atoi(m[1])
			py, _ := strconv.Atoi(m[2])
			shape = append(shape, [2]int{px, py})
		}
	}
	if id := named["mapid"]; id != "" && id != "0" && id != "-1" {
		return 0, 0, 0, nil, "map id " + id
	}
	points := slices.Clone(shape)
	if nx, okx := named["x"]; okx {
		if ny, oky := named["y"]; oky {
			px, errX := strconv.Atoi(nx)
			py, errY := strconv.Atoi(ny)
			if errX != nil || errY != nil {
				return 0, 0, 0, nil, "map x/y not numbers"
			}
			points = append(points, [2]int{px, py})
		}
	}
	if len(points) == 0 {
		return 0, 0, 0, nil, "map without a point"
	}
	var sx, sy float64
	for _, p := range points {
		sx += float64(p[0])
		sy += float64(p[1])
	}
	x = int(math.RoundToEven(sx / float64(len(points))))
	y = int(math.RoundToEven(sy / float64(len(points))))
	if v := named["plane"]; v != "" {
		plane, _ = strconv.Atoi(v)
	}
	a := mapArea(x, y, plane, named, shape)
	return x, y, plane, &a, ""
}

// mapArea is the area rule (entities.Area): the polygon the map draws, its
// rectangle or square around the point, else the default box around it.
func mapArea(x, y, plane int, named map[string]string, shape [][2]int) entities.Area {
	num := func(k string) (int, bool) {
		n, err := strconv.Atoi(named[k])
		return n, err == nil && n > 0
	}
	switch strings.ToLower(named["mtype"]) {
	case "polygon", "focusarea":
		if len(shape) >= 3 {
			a := entities.Area{Plane: plane, MinX: shape[0][0], MinY: shape[0][1], MaxX: shape[0][0], MaxY: shape[0][1], Polygon: shape, From: entities.AreaPolygon}
			for _, p := range shape {
				a.MinX, a.MaxX = min(a.MinX, p[0]), max(a.MaxX, p[0])
				a.MinY, a.MaxY = min(a.MinY, p[1]), max(a.MaxY, p[1])
			}
			return a
		}
	case "rectangle":
		w, okW := num("rectx")
		h, okH := num("recty")
		if okW && okH {
			return entities.Area{Plane: plane, MinX: x - w/2, MinY: y - h/2, MaxX: x + w/2, MaxY: y + h/2, From: entities.AreaRectangle}
		}
	case "square":
		if r, ok := num("r"); ok {
			return entities.Area{Plane: plane, MinX: x - r, MinY: y - r, MaxX: x + r, MaxY: y + r, From: entities.AreaSquare}
		}
	}
	return entities.PointArea(x, y, plane)
}

// buildLocationMaps derives the map point and area of each location.
func buildLocationMaps(raw rawLocationMaps) *entities.LocationMapCatalog {
	mc := &entities.LocationMapCatalog{}
	for _, l := range raw.Locations {
		if l.Map == "" {
			reason := l.Reason
			if reason == "" {
				reason = "no map"
			}
			mc.Unmapped = append(mc.Unmapped, entities.LocationUnmapped{ID: l.ID, Reason: reason})
			continue
		}
		x, y, plane, area, reason := mapPoint(l.Map)
		if reason != "" {
			mc.Unmapped = append(mc.Unmapped, entities.LocationUnmapped{ID: l.ID, Reason: reason})
			continue
		}
		mc.Maps = append(mc.Maps, entities.LocationMap{ID: l.ID, X: x, Y: y, Plane: plane, Area: area, Page: l.Page, RevID: l.RevID})
	}
	sort.Slice(mc.Maps, func(i, j int) bool { return mc.Maps[i].ID < mc.Maps[j].ID })
	sort.Slice(mc.Unmapped, func(i, j int) bool { return mc.Unmapped[i].ID < mc.Unmapped[j].ID })
	return mc
}
