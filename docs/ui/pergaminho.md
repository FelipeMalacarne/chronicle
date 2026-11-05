# Pergaminho (brand book, espelho do artefato de design system)

Pergaminho é o design system do **Ironman Planner**, o planner pessoal de uma conta ironman de Old School RuneScape. A linguagem visual é a da própria interface do jogo, sem copiar nenhum asset da Jagex: o papel do quest journal, a madeira e a pedra escura dos painéis, o bevel de uma aresta clara e outra escura, o amarelo sobre madeira, e o verde / amarelo / vermelho que todo jogador já lê como concluído / em progresso / não iniciado. Dois temas: **pergaminho** (claro, o padrão) e **pedra** (escuro, a interface transparente de noite).

## Quem usa e o que isso pede

Uma pessoa, jogando com o planner aberto ao lado do cliente ou no celular a um metro de distância. Daí:

1. **Uma hierarquia grita, o resto sussurra.** O passo atual é a única coisa grande na tela. Tudo o mais é lista densa e calma.
2. **Nunca mentir sobre certeza.** Todo check mostra de onde veio (hiscores, WikiSync, plugin, você) e quando. Dado velho parece velho (`stale`).
3. **O jogador manda.** A auto-detecção sugere, a cascata presume, o clique decide. Nada é desfeito pelo sistema sem aviso.
4. **Legível a um metro.** Texto do passo atual ≥ 22px no celular, contraste AA em todo par de cores, cor nunca como único sinal.
5. **Mesmos átomos em todo lugar.** A célula de skill, o slot de inventário, a linha de passo e o chip de entidade reaparecem idênticos em Agora, Rota, Metas, Atlas e Conta.

## Conteúdo e voz

- Interface em PT-BR, frases curtas, sentence case: "Fixar outro passo", "Levar do banco". Termos do jogo ficam em inglês (quest, diary, skill, nomes de itens e lugares).
- Texto dos passos é o do guia Oziris, sem tradução. Atribuição a OzirisLoL / ironman.guide em toda tela que mostra passos.
- Eyebrows em caixa alta mono: "PASSO ATUAL · 1.1.77", "LEVAR DO BANCO". Badges em caixa alta mono curta: "HS", "WS", "RL", "MANUAL".
- Estados dizem a causa: "falta 12 CON · 3 FLE", "presumido · 1.1.74 feito", "hiscores indisponível". Limitações ficam explícitas ("estimativa: ainda não"), nunca inventadas.
- Sem exclamação, sem emoji. Ícones de skill, quest e item vêm do OSRS Wiki com o rodapé "Imagens © Jagex, via Old School RuneScape Wiki, CC BY-NC-SA 3.0".

## Cor

Escada de superfícies: `ground` → `surface` → `elevated`, e por cima dela o `frame` (madeira/pedra escura) que carrega sidebar, barra do topo, placas e o botão primário. O frame é escuro nos dois temas: é o que faz o claro ainda parecer o jogo.

- **`gold` só sobre `frame`.** É o amarelo do jogo e some sobre pergaminho (2,3:1). Quando o mesmo significado precisa aparecer sobre o papel, use `gold-ink` (marrom-ouro de dia, o próprio gold de noite). Ouro é escasso: passo atual, meta fixada, item ativo da sidebar, texto do botão primário, nível na célula de skill.
- **Tricolor de quest.** `done` (verde), `progress` (amarelo = `gold-ink`) e `not-started` (vermelho) são os três estados que o quest journal ensinou. Use-os para quests, requisitos e passos; sempre com glifo (✓, ◐, cadeado) ou palavra.
- **`auto`** (azul) marca o que veio de fora: badges HS / WS / RL e a evidência de um check. Não existe azul no journal, por isso ele lê como "dado externo".
- **`stale`** (laranja queimado) é dado velho ou conflito. **`error`** é só falha de fonte ou de ação.
- Tints (`gold-tint`, `done-tint`, `auto-tint`, `stale-tint`) ficam por trás de badges e do realce do passo atual; palavras sobre tint usam `ink`.
- `border` e `hairline` são decorativos; o bevel (`shadow.bevel`, `shadow.bevel-frame`) é o que dá a sensação de painel do jogo: uma aresta clara em cima e à esquerda, uma escura embaixo e à direita, 1px cada, sem sombra externa.

Contraste verificado nos dois temas: `ink` ≥ 9:1, `ink-muted` ≥ 6:1, `ink-subtle` ≥ 4,6:1 sobre `elevated`, `gold` sobre `frame` 7,8:1, `done` / `not-started` / `auto` / `stale` ≥ 4,7:1 sobre `ground` e `surface`.

## Tipografia

Três famílias, todas hospedadas no Google Fonts (`Alegreya`, `Inter`, `JetBrains Mono`):

- **Alegreya** (`display`, `heading`, `plaque`) é a voz do jogo: uma serifa de pena como a do quest journal. Um `display` por tela (o passo atual); `plaque` em `gold` sobre `frame` nos títulos de painel.
- **Inter** (`body`, `control`, `label`, `caption`, `level`) é a interface. `level` usa numerais tabulares.
- **JetBrains Mono** (`eyebrow`, `data`, `qty`) para tudo que é valor: ids de passo, contagens, horários, quantidades.

Nada lido abaixo de 11px. Nenhuma fonte "RuneScape" no corpo: ela só caberia no logo, e não há logo.

## Espaço e forma

- Ritmo de 4px: `space-1` a `space-8`. Linhas de lista têm 36px no desktop e 52px no celular; alvos de toque ≥ `slot` (44px).
- A interface do jogo é quase quadrada: `radius-xs` (2px) é o padrão para slots, badges e placas; `radius-sm` (4px) para botões e chips; `radius-md` (6px) para cards. Nada de pílula.
- Profundidade vem do bevel, não de sombra. `shadow.panel` só para o que flutua (paleta ⌘K, menus, toasts).

## Átomos

- **Célula de skill**: slot de `frame` com bevel, sigla da skill em `on-frame-muted` (mono, 3 letras: CON, FLE, COO), nível em `gold` grande, e a linha "→ 20 · 1.1.81" em `on-frame-muted`. Atendida: a seta vira ✓ em `done`. Gargalo de meta: contorno `gold`.
- **Slot de inventário**: 44×44, `elevated` com bevel, quantidade mono no canto superior esquerdo (como no jogo), nome do item embaixo em `label`. Item faltando: contorno `not-started` e "falta".
- **Linha de passo**: glifo de estado (18px) · id mono · texto · badges · evidência. Estados: feito detectado (círculo cheio `done`, badge da fonte), feito manual (contorno `done`), presumido (tracejado `done` + "confirmar"), desatualizado (`done` dessaturado + ◷), conflito (contorno `stale` + "!"), atual (anel `gold-ink` + `gold-tint` na linha), pendente (contorno `border`), bloqueado (cadeado + "falta X"), pulado (traço + texto riscado).
- **Chip de entidade**: tipo em mono curto (QUEST, LEVEL, ITEM, DIARY, DESBLOQ.) + nome + ponto de estado no tricolor. O tipo é texto, nunca cor: cor é só estado.
- **Placa**: faixa de `frame` com bevel-frame e título `plaque` em `gold`. Abre o card do passo atual e os painéis laterais. Uma placa por painel.
- **Barra de progresso**: trilho `frame-2`, preenchimento `done` (rota, seção) ou `gold-ink` (meta), 6px, como a barra de XP.

## Componentes

Os átomos existem como componentes React (UMD, `window.Pergaminho`, React 18 via jsDelivr) em `components/bundle.js` + `bundle.css`, com tipos em `index.d.ts`:

- `Theme` envolve a tela e aplica `data-theme` (`pergaminho` | `pedra`).
- `Plaque` / `Panel`: placa de título e painel pronto.
- `SkillCell`: célula de skill com ícone, nível e alvo.
- `InventorySlot`: slot 44×44 com quantidade, `missing` e `unknown`.
- `StepRow`: linha de passo com os nove estados.
- `ObjectiveChip`: papel + veredito + chave canônica do objetivo.
- `BankCard`: com você / no banco / o guia manda.

Cada componente tem README e preview nos dois temas em `components/<Nome>/`. No app Svelte os mesmos nomes de classe (`pg-*`) e tokens valem sem React.

## Layout

Uma coluna até 720px, com o passo atual fixo no topo e barra inferior Agora · Rota · Metas · Mais. Conteúdo + painel de 300px acima de 1024px. Sidebar de `frame` com 232px acima de 1280px, com as fontes e o alternador de tema no rodapé. Conteúdo no máximo 1160px.

## Estados e movimento

- Botões: primário (`frame` + `gold`, bevel-frame), secundário (`elevated` + `ink`, bevel), ghost (transparente, `ink-muted`), perigo (`error` + `on-frame`). Hover escurece 6%; pressionado inverte o bevel; desabilitado a 45%.
- Foco: anel `focus` de 2px, com 2px de folga.
- Seleção: linha selecionada sobe para `elevated`; passo atual ganha `gold-tint` e anel `gold-ink`.
- Feito e Pular avançam a fila na hora e oferecem Desfazer por 8 s. Toque longo no glifo abre Feito / Pular / Reabrir / Fixar como atual / Nota.
- Movimento: 120ms para hover e toggles, 200ms para o card do passo avançar; texto nunca anima; `prefers-reduced-motion` deixa só fades.
