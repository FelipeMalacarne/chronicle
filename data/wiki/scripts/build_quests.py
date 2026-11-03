import json, re, glob, sys, html
bucket_path, pages_glob, guide_list, out_path = sys.argv[1:5]
rows = json.load(open(bucket_path))['bucket']
pages = {}
for f in sorted(glob.glob(pages_glob)):
    for p in json.load(open(f))['query']['pages']:
        if 'revisions' in p:
            pages[p['title']] = p['revisions'][0]['slots']['main']['content']

def tpl(text, name):
    """Return params dict of the first {{name|...}} template (top-level params only)."""
    m = re.search(r'\{\{' + re.escape(name) + r'\s*\|', text)
    if not m: return None
    i = m.end(); depth = 1; start = i
    while i < len(text) and depth:
        if text.startswith('{{', i): depth += 1; i += 2
        elif text.startswith('}}', i): depth -= 1; i += 2
        else: i += 1
    body = text[start:i-2]
    # split on | at depth 0 (ignoring [[...]] and {{...}})
    parts, cur, d1, d2 = [], '', 0, 0
    j = 0
    while j < len(body):
        c2 = body[j:j+2]
        if c2 == '{{': d1 += 1; cur += c2; j += 2; continue
        if c2 == '}}': d1 -= 1; cur += c2; j += 2; continue
        if c2 == '[[': d2 += 1; cur += c2; j += 2; continue
        if c2 == ']]': d2 -= 1; cur += c2; j += 2; continue
        if body[j] == '|' and d1 == 0 and d2 == 0: parts.append(cur); cur = ''; j += 1; continue
        cur += body[j]; j += 1
    parts.append(cur)
    out = {}
    for p in parts:
        if '=' in p:
            k, v = p.split('=', 1); out[k.strip()] = v.strip()
    return out

def strip_wiki(s):
    s = re.sub(r'\[\[([^\]|]*)\|([^\]]*)\]\]', r'\2', s)
    s = re.sub(r'\[\[([^\]]*)\]\]', r'\1', s)
    s = re.sub(r"'''?", '', s)
    return s.strip()

quest_names = {r['page_name'] for r in rows}
ALIAS = {
 "Recipe for Disaster/Another Cook's Quest": "Recipe for Disaster - Another Cook's Quest",
 'Recipe for Disaster/Defeating the Culinaromancer': 'Recipe for Disaster - Culinaromancer',
 'Recipe for Disaster/Freeing Evil Dave': 'Recipe for Disaster - Evil Dave',
 'Recipe for Disaster/Freeing King Awowogei': 'Recipe for Disaster - King Awowogei',
 'Recipe for Disaster/Freeing Pirate Pete': 'Recipe for Disaster - Pirate Pete',
 'Recipe for Disaster/Freeing Sir Amik Varze': 'Recipe for Disaster - Sir Amik Varze',
 'Recipe for Disaster/Freeing Skrach Uglogwee': 'Recipe for Disaster - Skrach Uglogwee',
 'Recipe for Disaster/Freeing the Goblin generals': 'Recipe for Disaster - Wartface & Bentnoze',
 'Recipe for Disaster/Freeing the Lumbridge Guide': 'Recipe for Disaster - Lumbridge Guide',
 'Recipe for Disaster/Freeing the Mountain Dwarf': 'Recipe for Disaster - Mountain Dwarf',
 'The Fractured Archive (miniquest)': 'The Fractured Archive',
 'Vale Totems (miniquest)': 'Vale Totems',
}
seen = set(); quests = []
for r in rows:
    name = r['page_name']
    if name in seen or name == 'Recipe for Disaster/Full guide': continue
    seen.add(name)
    req = r.get('requirements') or ''
    skill_reqs, quest_reqs, quest_reqs_all, other = [], [], [], []
    qp_req = None
    in_quest_block = False
    for line in req.split('\n'):
        if not line.strip(): continue
        depth = len(line) - len(line.lstrip('*'))
        plain = strip_wiki(re.sub(r'<[^>]+>', '', line.lstrip('*'))).strip()
        m = re.search(r'data-skill="([^"]+)" data-level="([^"]+)"', line)
        links = re.findall(r'\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]', line)
        qlinks = [l for l in links if l in quest_names and l != name]
        if m and depth <= 1:
            skill, lvl = m.group(1), m.group(2).replace(',', '')
            lvl = int(lvl) if lvl.isdigit() else lvl
            boostable = None
            if 'not boostable' in line: boostable = False
            elif 'boostable' in line: boostable = True
            entry = {'skill': skill, 'level': lvl, 'boostable': boostable,
                     'required_to_start': 'required to start' in line}
            if skill == 'Quest points': qp_req = lvl
            elif skill == 'Combat': entry['skill'] = 'Combat level'; skill_reqs.append(entry)
            else: skill_reqs.append(entry)
            continue
        if 'Completion of the following quests' in plain:
            in_quest_block = True; continue
        if qlinks:
            for q in qlinks:
                if q not in quest_reqs_all: quest_reqs_all.append(q)
            partial = bool(re.match(r'^(Started|Partial|Begun|Progress|Have started|Reached|Complete the .* portion)', plain, re.I)) or ' portion of ' in plain
            is_direct = ((in_quest_block and depth == 2) or (depth <= 1)) and not partial
            if partial and depth <= 1: other.append(plain)
            if is_direct:
                for q in qlinks:
                    if q not in quest_reqs: quest_reqs.append(q)
            if depth <= 1 and not re.match(r'^(Completion of|Completed) ', plain) and not in_quest_block:
                other.append(plain)
            continue
        if depth <= 1:
            in_quest_block = False
            if plain: other.append(plain)
    page = pages.get(name, '')
    info = tpl(page, 'Infobox Quest') or {}
    rewards = tpl(page, 'Quest rewards') or {}
    details = tpl(page, 'Quest details') or {}
    series = info.get('series')
    qp = rewards.get('qp')
    quests.append({
        'name': name,
        'number': info.get('number'),
        'difficulty': r.get('official_difficulty') or details.get('difficulty'),
        'length': r.get('official_length') or details.get('length'),
        'quest_points': int(qp) if qp and qp.isdigit() else (qp or None),
        'quest_points_required': qp_req,
        'series': strip_wiki(series) if series else None,
        'members': (info.get('members') or '').lower() == 'yes' if 'members' in info else None,
        'release_date': strip_wiki(info.get('release', '')) or None,
        'is_miniquest': '{{Infobox Miniquest' in page,
        'aliases': {'wikisync': ALIAS.get(name, name), 'runelite': ALIAS.get(name, name)},
        'skill_reqs': skill_reqs,
        'quest_reqs': quest_reqs,
        'quest_reqs_all': quest_reqs_all,
        'other_reqs': other,
        'start_point': strip_wiki(re.sub(r'<[^>]+>', '', r.get('start_point') or '')),
        'items_required_raw': r.get('items_required'),
        'enemies_raw': r.get('enemies_to_defeat'),
        'ironman_concerns_raw': r.get('ironman_concerns'),
        'requirements_raw': req,
        'wiki_url': 'https://oldschool.runescape.wiki/w/' + name.replace(' ', '_'),
    })
json.dump(quests, open(out_path, 'w'), indent=1, ensure_ascii=False)
print('quests written:', len(quests))
print('with skill reqs:', sum(1 for q in quests if q['skill_reqs']), 'with quest reqs:', sum(1 for q in quests if q['quest_reqs']), 'with qp:', sum(1 for q in quests if isinstance(q['quest_points'], int)), 'with number:', sum(1 for q in quests if q['number']))
# cross-check
guide = [l.strip() for l in open(guide_list) if l.strip()]
names = {q['name'] for q in quests}
low = {n.lower(): n for n in names}
miss = []
for g in guide:
    if g in names: continue
    cand = [n for n in names if g.lower() in n.lower() or n.lower() in g.lower()]
    miss.append((g, cand[:4]))
print('guide names:', len(guide), 'not matching:', len(miss))
for g, c in miss: print('  MISMATCH', repr(g), '->', c)
d = {q['name']: q for q in quests}
print(json.dumps({k: d['Dragon Slayer II'][k] for k in ('name','number','difficulty','length','quest_points','quest_points_required','series','members','release_date','skill_reqs','quest_reqs','other_reqs')}, indent=1)[:2500])
print(json.dumps({k: d['Cook\'s Assistant'][k] for k in ('name','number','difficulty','length','quest_points','series','members','release_date','skill_reqs','quest_reqs','other_reqs')}))
