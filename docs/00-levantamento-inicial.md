# Ironman Planner self-hosted — levantamento e plano

Data: 2026-11-03. **Histórico**: foi o primeiro levantamento, feito sem acesso às fontes. Está superado
pelos docs 02 (fontes verificadas), 03 (produto), 04 (arquitetura) e 09 (modelo de domínio); fica como
registro do ponto de partida e não é mantido.

## 1. O que descobri (e o que não consegui ver)

- **ironman.guide não é acessível desta sessão** (o domínio não está liberado na política de rede do ambiente, assim como `oldschool.runescape.wiki`, `secure.runescape.com`, `api.wiseoldman.net`, `templeosrs.com` e `sync.runescape.wiki`). Não encontrei repositório público com o código ou os dados do site. O único repo no GitHub que se diz "ironman.guide no RuneLite" é um template vazio.
- **Guias alternativos abertos existem.** O OSRS Wiki hospeda guias de ironman no namespace `Guide:` (ex.: *B0aty HCIM Guide V3*), licença CC BY-NC-SA. Há inclusive um plugin RuneLite (`OblongNoodle/IronGuide`) com esse guia inteiro hard-coded em Java, step a step. Serve como referência de granularidade dos passos.
- **Conteúdo do ironman.guide:** para migrar o guia você vai precisar ou (a) exportar o progresso/estrutura do próprio site (DevTools → Application → LocalStorage, e o JSON que o site carrega), ou (b) liberar o domínio `ironman.guide` na rede do ambiente para eu extrair a estrutura. Atenção à licença do conteúdo se você for publicar a sua versão; para uso pessoal self-hosted não há problema prático.

## 2. Fontes de dados para tracking automático

| Fonte | O que dá | Como | Observações |
|---|---|---|---|
| **Hiscores oficial (JSON)** | levels, XP, rank por skill; KC de bosses e atividades (clues, LMS, etc.) | `GET https://secure.runescape.com/m=hiscore_oldschool_ironman/index_lite.json?player=NOME` (há variantes `_hardcore_ironman` e `_ultimate`) | Sem autenticação. Não dá quests, diaries, nem itens. Rate limit agressivo: faça polling baixo (ex.: a cada 10 a 30 min) e cacheie. |
| **WikiSync (plugin RuneLite + API do wiki)** | quests (estado por quest), **levels**, achievement diaries (por task), combat achievements, music tracks, collection log | `GET https://sync.runescape.wiki/runelite/player/NOME/STANDARD` | **O wiki pede explicitamente para não usar essa API em projetos de terceiros** e diz que vai limitar. Use só como importador opcional, não como base. |
| **Plugin RuneLite próprio** | tudo que o cliente enxerga: varbits/varps (quests, diaries, unlocks), skills, inventário, banco, collection log, posição, itens equipados | Plugin Java (sideload ou Plugin Hub) que faz `POST` para o seu servidor | É o caminho robusto e 100% seu. O código do WikiSync (BSD-2) é o modelo: ele baixa um *manifest* com a lista de varbits para enviar e manda deltas. Os IDs de varbit por quest/diary estão em `chisel.weirdgloop.org/structs`. |
| **Wise Old Man API** | histórico de snapshots, ganhos, EHP/EHB, grupos | `https://api.wiseoldman.net/v2/players/NOME` | Bom para gráficos de progresso histórico sem você precisar guardar snapshots desde o dia 1. |
| **TempleOSRS** | collection log sincronizado (plugin), KC | API documentada no site deles | Alternativa para collection log se você não quiser plugin próprio. |
| **OSRS Wiki Bucket API** | dados estruturados: quests (requisitos de skill/quest, itens, dificuldade), itens, monstros, diaries | `https://oldschool.runescape.wiki/api.php?action=bucket&query=bucket('quest').select(...).run()` (sintaxe Lua, nomes em minúsculas) | É daqui que sai o **grafo de requisitos**. A API antiga (Semantic MediaWiki / `ask`) está deprecada. |
| **Wiki prices API** | preços GE | `https://prices.runescape.wiki/api/1.0/latest` | Irrelevante para iron, exceto para "vale a pena alchar?". |

Conclusão: skills e KC vêm do hiscores sem esforço; quests, diaries e unlocks precisam do cliente (plugin próprio, ou WikiSync enquanto durar); requisitos vêm do Bucket do wiki.

## 3. Modelo de dados proposto

Conteúdo do guia em arquivos versionados (YAML/Markdown no repo), estado do jogador no banco.

```yaml
# content/guides/main/sections/02-early-quests.yaml
id: early-quests
title: Early quests
steps:
  - id: restless-ghost
    text: "Fale com Father Aereck e complete The Restless Ghost"
    links: [wiki:The_Restless_Ghost]
    complete_when:            # regras avaliadas automaticamente
      - quest: The Restless Ghost
        state: FINISHED
  - id: fish-shrimps-20
    text: "Pesque shrimps até 20 Fishing, guarde 1 shrimp para Family Crest"
    complete_when:
      - skill: Fishing
        min_level: 20
    manual_ok: true           # pode marcar na mão mesmo sem a condição
  - id: varrock-easy-diary
    text: "Finalize a Varrock Easy diary"
    complete_when:
      - diary: {area: Varrock, tier: Easy}
    requires: [step:restless-ghost]
```

Tipos de condição para começar: `skill`, `quest`, `diary`, `diary_task`, `combat_achievement`, `boss_kc`, `collection_log_item`, `varbit` (escape hatch), `manual`.

Estado do jogador:

- `player_snapshot` (fonte, timestamp, JSON bruto) — guarda tudo que entrou, para reprocessar quando mudar regra.
- `step_progress` (step_id, status: auto/manual/skipped, completed_at, source).
- `goals` (ex.: "Barrows gloves", "Quest cape", "Fire cape") que viram alvo do planejador.

Grafo de requisitos: nós = quests, níveis de skill, diaries, itens/unlocks, steps; arestas = `requires`. Importado do Bucket (quest → skill/quest requirements) e complementado pelo guia. Sobre ele:

- **"O que falta para X"**: fechamento transitivo dos requisitos menos o que já está feito, ordenado topologicamente.
- **"O que destravou agora"**: diferença entre snapshots.
- Visualização com Cytoscape.js (ou React Flow / d3-dag), destacando feito, disponível e bloqueado.

## 4. Arquitetura sugerida

Objetivo: um `docker compose up` e pronto.

- **App web full-stack em TypeScript** (SvelteKit ou Next.js) com **SQLite** via Drizzle. Um container, um volume. Postgres só se você quiser multi-usuário sério.
- **Worker de sync** no mesmo processo: cron que puxa hiscores e Wise Old Man, importa WikiSync se habilitado, e reavalia `complete_when` de todos os steps a cada snapshot novo.
- **Endpoint de ingestão** `POST /api/ingest` com token, para o plugin RuneLite próprio (fase 2).
- **Conteúdo do guia em arquivos** no repo (`content/`), carregado no boot e validado por schema (Zod). Atualizar o guia vira PR, com diff legível.
- **Grafo**: calculado no servidor, servido como JSON; renderizado no front.
- **Import/export** do estado do jogador em JSON desde o dia 1, para nunca mais depender de localStorage.

Por que não copiar o modelo do ironman.guide (tudo no browser): o problema que você quer resolver é exatamente a persistência e o cruzamento com dados externos, e isso pede servidor.

## 5. Fases

1. **MVP (1ª semana de trabalho)**: repo, Docker, SQLite, login simples, guia em YAML (começar com um guia aberto do wiki ou com a estrutura exportada do ironman.guide), checklist manual persistente, sync de hiscores com auto-complete de steps por `skill`/`boss_kc`, import/export JSON.
2. **Tracking de quests/diaries**: importador WikiSync (opcional, com aviso), conditions `quest`/`diary`/`combat_achievement`. Em paralelo, começar o plugin RuneLite próprio copiando a abordagem do WikiSync (manifest de varbits + POST de deltas).
3. **Grafo de requisitos**: importador do Bucket do wiki (quests e diaries), modelo de nós/arestas, tela com Cytoscape, "o que falta para X".
4. **Planejamento**: goals, caminho recomendado, estimativa de tempo por step (XP necessário × taxas aproximadas), reordenação do guia por disponibilidade atual.
5. **Extras**: histórico via Wise Old Man, collection log, atualização do guia com conteúdo novo (ex.: Sailing e novos quests), notas por step, múltiplas contas.

## 6. Decisões que ficam com você

1. Stack: TypeScript full-stack como proposto, ou prefere outra (Go + HTMX, Laravel, etc.)?
2. Onde vai rodar (VPS própria, homelab, Raspberry) para dimensionar o Docker.
3. Conteúdo inicial do guia: exportar do ironman.guide, liberar o domínio na rede do ambiente para eu extrair, ou começar por um guia aberto do wiki?
4. Nome da conta ironman para testar o sync (precisa estar nos hiscores).
5. Quer já o plugin RuneLite na fase 2, ou aceita depender do WikiSync no começo?

## Fontes consultadas

- WikiSync plugin: https://github.com/andmcadams/WikiSync (constantes `MANIFEST_URL`/`SUBMIT_URL` em `WikiSyncPlugin.java`)
- Política do wiki sobre WikiSync: https://oldschool.runescape.wiki/w/RuneScape:WikiSync
- Bucket API: https://oldschool.runescape.wiki/w/RuneScape:Bucket e https://meta.weirdgloop.org/w/Extension:Bucket/Api
- Bucket de quests: https://oldschool.runescape.wiki/w/Bucket:Quest
- Projetos que consomem o endpoint de player do WikiSync: `AbFarid/OSRS-keep-up`, `crichmond1989/osrs-mcp`
- Guia B0aty como plugin: https://github.com/OblongNoodle/IronGuide
- Endpoints de hiscores por modo: `emersonds/runefetch` (`src/hiscores/hiscores.go`)
