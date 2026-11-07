# Live-test reference: timing, pathing and API gotchas

Measured in the dev client on client 2.6.52 while building `fm/blackjacker`. Re-check after
`mb-bump`.

## Clicks and ticks

- **`Rs2NpcModel.click(action)` is slow:** it goes through `Microbot.doInvoke`, which moves the
  natural mouse first: 350-600 ms per click. For anything that must land within a tick, build
  the same `NewMenuEntry` and send it with
  `Microbot.doInvokeQuick(entry, Rs2UiHelper.getActorClickbox(npc.getNpc()))` (~150-190 ms).
  Example: `fm/blackjacker/Blackjacking.quickClick` (options cached per NPC id, so the click
  does not wait on the client thread).
- **The game drops a click sent while the player is still finishing an action.** A pickpocket
  clicked on the target's fall animation, or a knock-out ~200 ms after a "glances" message,
  got no response at all. Time follow-ups from the chat message that ends the action, a tick
  after it, not from an animation.
- **Animations and messages are a tick apart.** An NPC's knock-out animation shows ~600 ms
  before the chat message; its attack animation can still be playing as it falls, so an
  animation alone cannot tell a success from a failure. Chat can.
- **Loop period.** Tick-critical steps need a short loop (50 ms) and their own early step in
  the priority list, ahead of healing and upkeep, so nothing else delays them.
- **HP and stats from events.** Subscribe to `StatChanged`/`HitsplatApplied` in the plugin and
  keep the values in the script: `Rs2Player.getBoostedSkillLevel` waits on the client thread,
  which is seconds per call on a throttled client.

## NPC pathing (lures, followers)

- NPCs move in **follow mode** (OSRS wiki, Pathfinding): each tick one step diagonally toward
  the target if free, else one step along one axis, else stand still. They never walk around a
  wall, so leading one needs short hops along tiles it can reach in a straight line, waiting
  for it to catch up after each.
- The player's pathfinder (`Rs2Walker.getWalkPath`) goes through doors and around corners; an
  NPC cannot. Do not use it to decide whether an NPC can follow.
- **Test it offline.** Dump the area with `mb tile x y 0` (the `canMove.*.live` flags) into a
  fixture under `src/test/resources/...`, simulate follow mode against it, and replay real
  failure spots as tests. See `fm/blackjacker/LeadPlanner`, `Walls`, `LeadPlannerTest` and
  `pollnivneach-north-walls.txt`. In game, build the same grid from the collision maps with
  `WorldArea.canTravelInDirection` on the client thread (`Walls.around`).

## Shops and world hops

- `Microbot.hopToWorld` is refused with "Please finish what you're doing before using the
  World Switcher" while a shop is open, and for about a tick after its widget closes: close,
  wait ~2 ticks, then hop. Count failed hops toward the limit or a refused hop loops forever.
- The hop leaves the world list tab open; switch back to the inventory tab before the next
  item action.
- `Login.getRandomWorld` is deprecated; filter `Microbot.getWorldService().getWorlds()` with
  `Rs2WorldUtil.isWorldAccessible` (and members only for members areas).

## Overlays

- Inject the **plugin** into the overlay and read the script through it. Injecting the script
  class gives Guice a second, idle instance.
- Values derived from elapsed time (XP/h) flicker when recomputed every frame; cache them for
  about a second.

## Game mechanics that matter for skilling bots

- Auto retaliate on turns a single hit into a real fight; turn it off
  (`Rs2Combat.setAutoRetaliate(false)`, varp `OPTION_NODEF` = 1 means off).
- Coin pouches stack, so "inventory full" only blocks a pickpocket when there is no pouch in
  the bag.
