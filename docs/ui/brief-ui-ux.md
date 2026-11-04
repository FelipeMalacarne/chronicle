# Brief de UI/UX para o agente de design

Data: 2026-11-04. Para um agente de UI/UX que vai **revisar e reestruturar as telas no Claude Design
primeiro**. A implementação em `web/` vem em iterações futuras e não faz parte desta tarefa.

Canvas: **Ironman Planner — Telas**, https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ (páginas v0 a v3).

## 0. Como trabalhar

1. **Pergunte antes de desenhar.** O dono não está satisfeito com o canvas nem com a UI do código, mas
   ainda não disse por quê. Comece perguntando, com opções concretas. As hipóteses da seção 7 são um
   bom ponto de partida para as perguntas. Pergunte também qual direção visual seguir (seção 6).
2. **Desenhe com dados reais de conteúdo.** Use passos, seções, segmentos e loadouts reais dos dois guias
   (seção 4). Dados da conta (níveis, banco, nome) podem ser fictícios, mas marcados como tal. Não
   invente passos de guia: o layout atual do código falhou justamente por ter sido feito sobre fixtures
   que não parecem com o conteúdo real.
3. **Não apague as páginas v0–v3.** Elas são o histórico de decisões. Trabalhe numa página nova (`v4`),
   reaproveitando o que funcionou (seção 5).
4. **Não mexa no código** (`web/`, Go) nem nos docs numerados. Ao terminar, entregue no chat uma lista
   das decisões de UI tomadas com o dono, para virar documento depois.
5. Leia antes: `docs/09-modelo-de-dominio.md` (modelo de dados que a UI deve refletir), `docs/03-produto-e-features.md`
   (features e telas originais) e `docs/04-arquitetura.md`, seções 6.4.1 e 9. Este brief resume o que importa
   deles para a UI.

## 1. Produto e momentos de uso

Planner self-hosted para **uma** conta Ironman de OSRS (o dono). Ele segue **duas rotas independentes**:

- o **Oziris** Ironman Efficiency Guide (OzirisLoL / ironman.guide), com 576 passos curtos;
- o **BRUHsailer** (So Iron BRUH e ParasailerOSRS), com 226 passos longos.

O progresso é detectado sozinho (hiscores, Wise Old Man, plugin próprio no Microbot que lê skills,
quests, banco, inventário e equipamento).

Momentos de uso, em ordem de importância:

1. **Jogando** (zaros, cliente aberto ao lado ou celular como segunda tela): "o que faço agora, o que
   levo do banco, já fiz isso?". Precisa de pouca leitura, ações rápidas, atualização sozinha quando o
   plugin detecta algo.
2. **Planejando fora do jogo**: escolher metas (Barrows gloves, Quest cape), ver o que bloqueia, comparar
   as rotas, decidir qual seguir.
3. **Conferindo a conta**: skills, quests, banco, de onde veio cada dado e quão velho ele está.
4. **Controle do Microbot** (doc 08): módulo separado e desligado por padrão; fica fora desta rodada,
   exceto como ponto de entrada.

UI em PT-BR; termos do jogo em inglês (nomes de quest, skill, item, diary).

## 2. O modelo de dados que a UI precisa deixar visível (doc 09)

| Conceito | O que é | Como deveria aparecer |
|---|---|---|
| **Rota** (guia) | Ordem de eficiência de um autor. Oziris e BRUHsailer são independentes: nunca mesclar, sobrepor ou concatenar | Seletor de rota ativa; atribuição do autor sempre visível onde há passos |
| **Seção** | Capítulo da rota (Oziris 1.1, BRUHsailer `bruh-1.1`) | Navegação e progresso por seção |
| **Segmento** | Trecho entre bancos (`bank_leg`) ou sem banco (`sequence`), com ações de banco e loadout | Agrupador natural: "o que levar" vale para o trecho inteiro, não para um passo |
| **Passo** | Unidade de **navegação**: texto do autor | Linha ou card legível; no BRUHsailer é um parágrafo (seção 4) |
| **Objetivo** | Unidade **rastreável**: entidade + estado-alvo (`quest:x_marks_the_spot@finished`, `skill:agility>=70`, `item:960x120`, `unlock:fairy_rings@done`). Papel no passo: `completes`, `starts`, `requires`, `uses` | Chips sob o passo, com estado (✓/○/?) e evidência; o mesmo objetivo aparece igual nas duas rotas |
| **Entidade** | Coisa do mundo com id canônico: quest, item, skill, diary, CA, boss, **unlock**, **location** | Nome legível, ícone por tipo, link para a wiki, página/painel da entidade |
| **Progresso compartilhado** | Fatos da conta valem para as duas rotas; marca manual e posição (cursor) são por rota | Ao trocar de rota, o que já foi feito aparece feito; a posição de cada rota é preservada |
| **Status do passo** | `auto_done` (objetivos cumpridos, com evidência), `manual_done`, `implied` (cascata: só na seção e na rota ativa, aguardando confirmação), `skipped`, `pending`, bloqueado (requisito não cumprido), `regressed` | Forma, não só cor; evidência (fonte e hora) acessível; "implícito" claramente diferente de "feito" com ação "confirmar tudo" |
| **Veredito** | `met` / `unmet` / **`unknown`** | **`unknown` não é `unmet`**: "não sabemos" (sem plugin, sem leitura) precisa de visual próprio |
| **Frescor** | Cada container de itens é `live`, `restored` (espelho salvo, possivelmente velho) ou `unavailable`; cada fonte tem idade | `restored` sempre visível como desatualizado; idade da fonte ao lado de dados derivados |
| **Posse × setup × ação de banco** | Três perguntas distintas: "tenho o item?" (qualquer container), "estou carregando o setup do trecho?" (inventário/equipamento ao vivo), "o que o guia manda fazer no banco?" (instrução, nunca concluída) | Não fundir: "no banco 1", "com você ✓", "deposite tudo" são coisas diferentes |
| **Readiness do trecho** | `ready`, `listed_ready` (o guia lista o setup só em parte), `not_ready`, `unknown`, `no_loadout` | Resumo do próximo banco; nunca dizer "pronto" quando o guia lista o setup só em parte |
| **Meta** | Um objetivo escolhido como alvo; plano = fechamento de requisitos no grafo | Lista com gargalo, % e os passos de cada rota que levam a ela |

## 3. O que a API oferece hoje e o que está planejado

**Existe hoje:**
- `GET /api/guides`, `/api/guides/{g}`, `/api/guides/{g}/sections/{s}` (com segmentos e entidades
  por passo), `/api/guides/{g}/steps/{id}`;
- `GET /api/guides/{g}/readiness?account=&step=`: trecho atual e próximo bank leg, com status, itens
  (carregado × possuído), frescor por container e ações de banco. Só em modo dev.

**Planejado (doc 09):** objetivos por passo, `objective_progress`, marcas e cursor por rota, metas,
grafo, unlocks e locais.

Até a extração dos guias por agente (doc 09, seção 6) rodar:
- **o BRUHsailer não tem objetivos nem locais**;
- **o Oziris tem objetivos só onde havia regras** (209 de 576 passos).

Desenhe para o estado-alvo, mas com um caminho honesto para o estado atual ("sem objetivos detectáveis
neste passo: marque à mão").

## 4. Como o conteúdo real é (desenhe com isto)

**Oziris**: 7 seções, 576 passos, média de **53 caracteres** por passo.

```
1.1.5   Buy a spade, start X marks the spot quest
        objetivos: quest X Marks the Spot @ started (regra explícita) · usa: Spade
1.1.75  Run to Draynor bank
        trecho de banco com 41 passos; loadout parcial: GP, Bronze nails, Hammer, Saw,
        120 noted planks, 1k arrow shafts (não resolvido), 1k bronze arrowheads (não resolvido),
        1 house tab
```

- Tem `location` em muitos passos (120 locais distintos) e passos `enhanced` (camada do ironman.guide).
- Tem notas (`tip`, `hcim`, `update`, `warning`) e uma seção paralela (`S`, Sailing) do tipo track.

**BRUHsailer**: 9 seções, 226 passos, média de **1.097 caracteres** por passo (o maior tem 7.492).
Um passo real (`bruh-1.1-698825e36f25`, referência do autor `CH1.6`) começa assim:

```
- TL;DR: do Clock Tower, Monk's Friend, Hazeel Cult and get level 35 woodcutting, burning the logs
  and swapping to oaks at 15. Eat your food if you take damage. …
  - Run to the clocktower, talk to Brother Kojo to start Clock Tower. Go down the ladder and start the
    steps for the white cog (north-west direction), grabbing a second rat poison along the way. …
  (dezenas de sub-bullets)
notas: GP after step: 2500 gp · Items needed: jug of water, 1 normal log, steel axe, tinderbox ·
       Total time: 3 hrs 25 minutes
```

Consequências para a UI:

- **Uma linha de passo de uma linha não serve para o BRUHsailer.** O passo dele precisa de anatomia
  própria:
  - o TL;DR como título;
  - os sub-bullets recolhíveis, sem perder a leitura;
  - GP, tempo acumulado e itens como metadados;
  - vários objetivos por passo (dezenas, depois da extração), com progresso parcial dentro do passo.
- **Ids internos do BRUHsailer são hashes** (`bruh-1.1-698825e36f25`) e nunca devem aparecer. Mostre a
  referência do autor (`CH1.6`) ou a posição. O Oziris pode mostrar o id do autor (`1.1.75`).
- **Locais**: o Oziris tem; o BRUHsailer só terá depois da extração.
- **Bancos**: Oziris tem 32 segmentos (25 bank legs); BRUHsailer tem 102 (99 bank legs), quase um banco
  a cada passo.

**Onde buscar mais amostras**: `content/guides/oziris-main/guide.json` e
`content/guides/bruhsailer-main/guide.json` (variants → sections → segments → steps), e
`entities.report.json` de cada guia (o que não resolveu). Com o servidor rodando:
`/api/guides/{g}/sections/{s}`.

## 5. O que já existe (aproveite e critique)

**Canvas**:

| Página | O que trouxe | Para levar adiante |
|---|---|---|
| v0 · telas atuais | Agora, Checklist, Conta, Grafo, Meta, Microbot; tema Senntisten (Catppuccin Mocha) | Legenda de estados de passo (nota `s1`) |
| v1 · planner por entidades | Tudo é entidade; relação requer → destrava; **Atlas** com foco; ⌘K; meta = entidade fixada; rota como propriedade, não aba; comparação virou referência cruzada na entidade | Atlas por foco, ⌘K, entidade como centro (bate com o doc 09) |
| v2 · Forja | Fundamentos (cor, tipo, átomos, estados); ouro só onde há decisão; átomo "célula de skill" repetido | Disciplina de cor e o átomo reutilizado |
| v3 · Pergaminho | Pele do jogo (papel de dia, pedra de noite, madeira, tricolor verde/amarelo/vermelho de quest); chips de entidade sob o passo; passos agrupados por local; "presumido · confirmar"; evidência com fonte e hora; "disponível fora da rota"; recorrentes; Feito/Pular/Desfazer com J/K | Quase todo o modelo de interação; a decisão de tema fica com o dono |

**Código** (`web/`, Svelte; telas `/plan` e `/compare`; tema Senntisten):

- **Seletor de guia** no topo; `/plan` com card "Agora", checklist por seção e card "Próxima perna de
  banco".
- **`/compare`** com duas faixas ligadas só por marcos de quest compartilhados (Monk's Friend, Sheep
  Herder, Sea Slug…).
- **Problemas observados**:
  - a home (`/`) está quebrada (página de erro 500);
  - a navegação tem sete itens sobrepostos (Agora, Plano, Guia, Comparar, Conta, Metas, Grafo);
  - os passos são truncados numa linha;
  - o progresso é zero (ainda não há API);
  - o estado da sincronização aparece como um "aguardando" pouco claro.

## 6. Direção visual: decisão do dono

Há três linguagens no canvas:

- **Senntisten** (Catppuccin Mocha; é a usada no código);
- **Forja** (aço e ouro);
- **Pergaminho** (pele do jogo, com tema claro e escuro; design system próprio instalado no canvas).

Não escolha sozinho: mostre o mesmo artboard-chave nas candidatas que o dono quiser comparar e
pergunte. Os design systems Senntisten e Pergaminho estão instalados no canvas
(`project/ds/senntisten`, `project/ds/pergaminho`).

## 7. Lacunas que esta rodada deve resolver (hipóteses para confirmar com o dono)

1. **O BRUHsailer não existe no canvas.** Rota, passo-parágrafo e troca de rota precisam ser desenhados
   com ele.
2. **Comparação de rotas.** A v1 a substituiu por "referência cruzada na entidade"; o código tem
   `/compare` em faixas. Decidir com o dono: tela própria, painel na entidade, ou os dois. Com o
   progresso compartilhado, "onde cada rota faz este objetivo" e "quanto cada rota já está coberta pelo
   que eu fiz" ficam naturais.
3. **Objetivos como cidadãos de primeira classe.** Chips sob o passo existem na v3, mas falta:
   - progresso parcial de um passo com muitos objetivos;
   - o papel do objetivo no passo (`completes` × `requires` × `uses`);
   - o estado `unknown`.
4. **Status `implied` só na rota ativa.** Um objetivo feito em outra rota aparece como feito, mas não
   gera "presumido" na rota que não está ativa.
5. **Cursor por rota**: trocar de rota mantém o lugar; mostrar onde você está em cada uma.
6. **Readiness do trecho e frescor.**
   - "Levar do banco" precisa distinguir: carregando agora, no banco (ao vivo ou restaurado),
     não verificável, e o que o guia não especifica.
   - O BRUHsailer tem um banco quase a cada passo: repensar o card de banco para esse ritmo.
7. **Estados sem dados**: sem plugin, sem conta, banco restaurado, fonte velha, passo sem objetivos
   detectáveis, item não resolvido pelo catálogo.
8. **Navegação enxuta.** Proposta para validar:
   - **Agora**: cockpit da rota ativa;
   - **Rota**: o guia inteiro, com seletor de rota;
   - **Metas**;
   - **Atlas**: entidades e grafo;
   - **Conta**: skills, quests, banco e fontes;
   - **Microbot**: só se habilitado.

   A comparação é discutida no item 2.
9. **Celular como segunda tela** durante o jogo: Agora e Rota legíveis com uma mão, passo-parágrafo do
   BRUHsailer incluído.

## 8. Restrições

- **Atribuição** visível em toda tela com passos: OzirisLoL / ironman.guide e So Iron BRUH e
  ParasailerOSRS. Imagens do jogo só da wiki, com crédito (CC BY-NC-SA).
- **Acessibilidade**:
  - estado por forma e cor, nunca só cor;
  - contraste 4,5:1;
  - alvos de toque ≥ 44 px;
  - botões e links reais.
- **Performance futura**: rotas com centenas de passos. Prever seções recolhidas e listas longas sem
  montar tudo.
- **Realismo técnico**:
  - SPA estática servida pelo binário Go;
  - não depender de recursos que a API não terá (seção 3);
  - a UI nunca conclui passo sem evidência;
  - ações de banco nunca são marcadas como feitas.
- **Nada de dados inventados de conteúdo.** Placeholders explícitos onde faltar dado da conta.

## 9. Entregáveis desta rodada

1. Respostas do dono às perguntas iniciais (o que não gostou, direção visual, comparação, escopo).
2. Página **v4** no canvas com, no mínimo:
   - **Agora** desktop, uma vez com Oziris e uma com BRUHsailer como rota ativa;
   - **Rota** com o passo-parágrafo do BRUHsailer e o passo curto do Oziris;
   - **Comparação** (na forma decidida);
   - **Agora no celular**;
   - uma prancha de **fundamentos e estados**: status de passo, veredito `unknown`, frescor, chip de
     objetivo, card de banco.
3. Notas no canvas explicando as decisões, com cada tela usando dados reais dos guias.
4. No chat, a lista de decisões de UI para virar documento (`docs/`) antes da implementação.

## 10. Prompt sugerido para iniciar o agente

```text
Você vai revisar e reestruturar as telas do Ironman Planner no Claude Design. Leia primeiro
docs/ui/brief-ui-ux.md e depois docs/09-modelo-de-dominio.md neste repositório. O canvas é
https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ. Antes de desenhar qualquer coisa, me pergunte o
que não me agrada no canvas e na UI atual, qual direção visual seguir e como tratar a comparação de
rotas (seções 0, 6 e 7 do brief). Trabalhe numa página nova v4, com dados reais dos guias, sem apagar
as páginas antigas e sem mexer no código. Pergunte sempre que precisar.
```
