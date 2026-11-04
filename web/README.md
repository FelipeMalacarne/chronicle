# web — SPA do Ironman Planner

SvelteKit (Svelte 5, runes) + TypeScript + Vite, sem framework de CSS. O build sai em `web/dist`
como SPA (`index.html` de fallback) e é embutido no binário Go por `web/embed.go`.

## Comandos

```sh
pnpm install
pnpm dev        # http://localhost:5173, /api vai por proxy para http://localhost:8080 (API_TARGET para trocar)
pnpm dev:mock   # idem, mas /api é servido do ../data/guide/normalized/guide.json (sem Go)
pnpm check      # svelte-check
pnpm lint       # prettier --check + eslint
pnpm format     # prettier --write
pnpm build      # gera web/dist (apaga o conteúdo anterior, exceto o que o Go espera)
pnpm preview    # serve o dist localmente
```

## Onde fica o quê

- `src/lib/styles/tokens.css`: tokens do design system Senntisten Shell (Catppuccin Mocha) como
  variáveis CSS, reset e utilitários (`.card`, `.btn`, `.chip`, `.badge`, `.eyebrow`...).
- `src/lib/api/types.ts`: tipos do contrato HTTP do núcleo Go.
- `src/lib/api/client.ts`: cliente fetch (`api.now()`, `api.guide()`, `api.section(slug)`,
  `api.step(id)`, `api.health()`), `ApiError` e o indicador `sync` (ok / stale / offline) usado na
  barra superior. O estado reativo vive em `sync.svelte.ts`.
- `src/lib/api/rules.ts`: deriva badges (`firemaking ≥ 50`, `quest FINISHED`), marcos de skill e
  agrupamento por local a partir de `complete_when`.
- `src/routes/`: `/` Agora, `/guide` seções, `/guide/[section]` checklist, `/account`, `/goals`,
  `/graph` (placeholders).

## Proxy e mock

SvelteKit 3 não lê mais `svelte.config.js`: adapter e demais opções ficam em `sveltekit({...})` dentro
de `vite.config.ts`. Em dev, o mesmo arquivo encaminha `/api` para `API_TARGET` (padrão `http://localhost:8080`).
Com `MOCK_API=1` um middleware do Vite responde o contrato direto do JSON normalizado, com
progresso fictício em `/api/now`. Em produção não há proxy: o Go serve `dist` e `/api` na mesma
origem, então o cliente usa só caminhos relativos.
