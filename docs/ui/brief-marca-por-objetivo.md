# Brief de UI/UX: marcar um objetivo como já feito

Data: 2026-11-14. Para o agente de design, que desenha **no canvas primeiro**. A implementação em `web/`
vem depois, do que o dono aprovar. Canvas: **Ironman Planner — Telas**,
https://claude.ai/artifact/NqmKesmqCJH1gnSowjJFbQ, página **8 · v7 · Frota**.

## 0. Como trabalhar

1. **Pergunte antes de desenhar**, com opções concretas (seção 6). O dono já usou a versão mínima e tem
   opinião formada.
2. **Desenhe com passos reais do BRUHsailer** (seção 3), nunca com dados inventados.
3. **Não edite as artboards v7 existentes.** Trabalhe numa página nova ou numa artboard nova
   (`V7MarcaObjetivo1440`, `V7MarcaObjetivo640`) e reaproveite as peças de `V7Fundamentos`
   ("objetivo com progresso", cartão, barra de andamento). Mantenha as notas do dono no canvas.
4. **Não mexa em código.** No fim, entregue no chat as decisões de UI tomadas com o dono, para virar
   `docs/ui/decisoes-canvas.md`.

## 1. Por que isto existe

O motor só fecha um passo sozinho (`auto_done`) quando **todos** os objetivos que ele conclui estão
cumpridos **com evidência** (hiscores, plugin). Os passos do BRUHsailer são parágrafos com dezenas de
ações. Depois da revisão de 2026-11-13, eles têm objetivos de item como "Blue cog", "Black cog",
"Red cog", "Child's blanket" (Clock Tower, CH1.6) ou "110 Logs" (CH1.1).

Item gasto não se prova depois: o jogador entregou as engrenagens, queimou os logs. O plugin não vê mais
o item, então o objetivo fica `unknown`, o passo volta a `pending` e o cursor recua. Foi exatamente isso
que aconteceu com o dono: o cursor foi até o CH1.2 porque ele "não tem mais as engrenagens".

A saída decidida (doc 09 seção 5, decisão 38): o dono **marca o objetivo como já feito**. A marca é da
conta inteira, vale em qualquer rota e passo, **não é evidência** e nunca vira `met` no banco: ela
sobrepõe o veredito na hora de derivar o status. Desmarcar restaura o veredito do motor.

## 2. O que já existe (versão mínima, fora do canvas)

Feita com autorização do dono em 2026-11-13, em `web/src/lib/v6/GoalGrid.svelte`:

- Objetivo **não cumprido** (`unmet` ou `unknown`): botão secundário "Já fiz" ao lado do cartão.
- Objetivo **marcado**: mostra `feito`, o selo "marcado por você" e o botão "Desfazer".
- Objetivo **cumprido por evidência**: sem controle.
- Toast "Marcado · Knife marcado como já feito" com `desfazer` (tecla `U`), na pilha de desfazer dos passos.
- Só aparece no passo atual (`/agora`) e no passo aberto da rota (`/rotas/...`). Os chips pequenos das
  listas não têm ação.

Os rótulos, o lugar do botão e o visual são palpite de quem implementou. **Critique e substitua.**

## 3. Dados reais para desenhar

**Passo atual, CH1.6 (Clock Tower, Monk's Friend, Hazeel Cult, woodcutting 35)**, objetivos de hoje:
quest Clock Tower iniciada e concluída, Monk's Friend concluída, Hazeel Cult concluída, Woodcutting 35,
Firemaking 39, Child's blanket, Blue cog, Black cog, Red cog. É um passo em que o dono marca 4 itens que
já entregou e deixa os outros para o motor provar.

**Passo CH1.1**, objetivos: Knife, Jug of water, 2 Bucket, Cabbage, Leather boots, Rune Mysteries
(iniciada), 15 Firemaking, 110 Logs. Todos `unknown` numa conta sem leitura do plugin.

**Passo CH1.18 (Wintertodt)**: 22 objetivos, 3 skills e 19 itens (50 uncut rubies, 16 magic logs, 128k gp,
200 pure essence...). É o caso de **muitos objetivos para marcar de uma vez**.

Estados a cobrir: `met` por evidência, `met` marcado por você, `unmet`, `unknown` (nunca mostrar `unknown`
como `unmet`), item com quantidade ("110 Logs", "2 Bucket"), e item com progresso parcial.

## 4. O que precisa ser desenhado

1. **O controle no objetivo.** Onde fica, como se chama, como se diferencia de "Feito" do passo (que
   marca o **passo** inteiro e é outra coisa). Hoje há dois botões com sentidos próximos na mesma tela.
2. **O estado "marcado por você"** no cartão do objetivo: sem evidência, mas contando como feito. Tem que
   ser distinguível de `met` por evidência sem virar texto de sistema em parágrafo (explicações vão no
   `title`).
3. **Marcar vários de uma vez.** Passos como o CH1.18 têm 19 itens. Marcar um a um não escala:
   "marcar todos os itens que faltam", seleção múltipla, ou "já fiz todos os itens deste passo".
4. **Desfazer**, no cartão e no toast. E o que o dono vê quando a marca muda o cursor (o cursor volta ao
   lugar certo: avisar quantos passos fecharam).
5. **Onde ver tudo o que eu marquei.** Hoje não há lista. Candidatos: aba Histórico (suas ações, como
   as marcas de passo), seção na Visão geral, ou filtro na Rota ("com marca minha").
6. **Estados de conflito:** o objetivo marcado e o plugin depois prova que está `unmet` (a marca vence até
   desmarcar); o objetivo marcado e depois provado `met` (a marca fica inofensiva). O dono precisa ver
   isso sem confusão.
7. **A linha do passo nas listas** (Rota inteira, Em seguida): mostrar que o passo fechou por marca de
   objetivo e não por evidência, como já se distingue "feito" de "implícito".

## 5. Restrições

- Sistema de design **Pergaminho**: tokens em `docs/ui/pergaminho.tokens.json`, classes `pg-*`. Um só
  bisel (painel, placa, controles), tudo dentro dos painéis chapado.
- **Sem texto de sistema em parágrafo.** Explicações vão no `title`.
- `unknown` nunca aparece como `unmet`.
- Atribuição do guia (So Iron BRUH e ParasailerOSRS) segue visível em toda tela com passos.
- Sprites reais do OSRS Wiki, com o crédito CC BY-NC-SA 3.0.
- Desenhar **1440** (com a sidebar) e **640** (janela ao lado do jogo). A janela de 640 px é onde o dono
  mais marca, enquanto joga: o controle tem que caber e ter alvo de toque razoável.
- Atalhos de teclado já usados na tela do passo: `J` Feito, `K` Pular, `U` Desfazer. Não colidir.

## 6. Perguntas para o dono (faça com opções concretas)

1. O botão por objetivo deve ser **sempre visível** ou só ao passar o mouse e ao focar?
2. "Marcar todos os itens que faltam" do passo: quer isso? Marcaria também skills e quests, ou só itens?
3. Quando você marca o último objetivo que faltava, o passo deve **fechar na hora** ou pedir um "Feito"?
4. Quer uma **lista única** das suas marcas (para revisar e desfazer em bloco)? Onde: Histórico ou Conta?
5. Quando o plugin voltar a ver o item e provar `met`, a marca deve **sumir sozinha** ou ficar até você
   desfazer?

## 7. Entregáveis

- Artboards 1440 e 640 do passo atual com o controle, os 4 estados do objetivo e o toast.
- Artboard do passo com muitos objetivos (CH1.18) com a marcação em bloco.
- Artboard da lista "suas marcas", se o dono quiser.
- Lista das decisões tomadas com o dono, no chat.

## 8. Referências

- `docs/09-modelo-de-dominio.md` seção 5 (a marca sobrepõe o veredito) e `docs/01-sumario-executivo.md`
  decisão 38.
- `docs/ui/decisoes-canvas.md` seção 7 (v7) e a nota de dívida de 2026-11-13.
- `web/src/lib/v6/GoalGrid.svelte` (versão mínima atual) e `web/src/lib/progress/undo.ts` (pilha de
  desfazer).
- API: `PUT /api/objectives/mark`, `POST /api/objectives/unmark`; o board traz `owner_marked`. CLI:
  `chronicle objective mark|unmark`.
