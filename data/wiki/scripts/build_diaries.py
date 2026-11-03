import json,re,glob,sys
src_glob,out=sys.argv[1:3]
def strip(s):
    s=re.sub(r'<ref[^>]*>.*?</ref>','',s,flags=re.S); s=re.sub(r'<ref[^>]*/>','',s)
    s=re.sub(r'\{\{sic\}\}','',s); s=re.sub(r'\[\[([^\]|]*)\|([^\]]*)\]\]',r'\2',s); s=re.sub(r'\[\[([^\]]*)\]\]',r'\1',s)
    return re.sub(r"'''?",'',s).strip()
res=[]
for f in sorted(glob.glob(src_glob)):
    t=json.load(open(f))['parse']['wikitext']
    for m in re.finditer(r'data-diary-name="([^"]+)" data-diary-tier="([^"]+)"(.*?)\n\|\}',t,re.S):
        area,tier,body=m.groups(); tasks=[]
        for row in re.split(r'\n\|-\s*\n',body)[1:]:
            cells=re.split(r'\n\|',('\n'+row).rstrip('\n'),maxsplit=2)[1:]
            if not cells: continue
            tm=re.match(r'\s*(\d+)\.\s*(.*)',cells[0],re.S)
            if not tm: continue
            num,desc=tm.groups(); desc=desc.split('\n')[0]
            reqtxt=cells[1] if len(cells)>1 else ''
            skills=[{'skill':a,'level':int(b.replace(',',''))} for a,b in re.findall(r'\{\{SCP\|([A-Za-z ]+)\|([\d,]+)',reqtxt)]
            quests=re.findall(r'(?:Completion of|Completed|Started|Partial completion of)\s+\[\[([^\]|]+)',reqtxt)
            tasks.append({'n':int(num),'task':strip(desc),'skill_reqs':skills,'quest_reqs':quests,'requirements_raw':reqtxt.strip()})
        res.append({'area':area,'tier':tier,'task_count':len(tasks),'tasks':tasks})
json.dump(res,open(out,'w'),indent=1,ensure_ascii=False)
print(len(res),'area/tier blocks;', sum(r['task_count'] for r in res),'tasks')
print(json.dumps(res[0]['tasks'][:2],indent=1,ensure_ascii=False))
