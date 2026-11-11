package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// A run waits for a safe point before it touches anything (docs/08 phase 4,
// delivery 3 gap): it never walks the character out of a minigame, a raid or
// a boss room, out of a dialogue, a fight or a break. Without the safe point
// the trigger would act the moment the kitten grows, wherever the character is.

// runSafeWait is how long a run waits for the safe point by default.
const runSafeWait = time.Hour

// unsafe returns why the character is not at a safe point, or "".
func (e *Executor) unsafe(ctx context.Context) string {
	st, err := e.runState(ctx)
	switch {
	case err != nil:
		return "sem resposta do cliente"
	case !st.LoggedIn || st.Player.Position == nil:
		return "fora do jogo (break ou logout)"
	case st.Player.HealthRatio != nil && *st.Player.HealthRatio >= 0:
		// The health bar shows only during and shortly after a fight.
		return "em combate"
	}
	if name, ok := unsafeRegions[region(*st.Player.Position)]; ok {
		return "em " + name
	}
	// /dialogue is the Agent Server's; an older one without it skips the check.
	if raw, err := e.Game.JSON(ctx, http.MethodGet, "/dialogue", nil, nil, 10*time.Second); err == nil {
		var d struct {
			InDialogue bool `json:"inDialogue"`
		}
		if json.Unmarshal(raw, &d) == nil && d.InDialogue {
			return "num diálogo"
		}
	}
	return ""
}

// waitSafe waits until the character is at a safe point, noting each new
// reason it is not on the trail.
func (e *Executor) waitSafe(ctx context.Context) error {
	limit := e.SafeWait
	if limit <= 0 {
		limit = runSafeWait
	}
	noted := ""
	for deadline := time.Now().Add(limit); ; {
		why := e.unsafe(ctx)
		if why == "" {
			if noted != "" {
				e.note(RunWaiting, "ponto seguro")
			}
			return nil
		}
		if why != noted {
			e.note(RunWaiting, "esperando: "+why)
			noted = why
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no safe point within %d min: %s", int(limit.Minutes()), why)
		}
		if !e.sleep(ctx) {
			return ctx.Err()
		}
	}
}

// region is the map region of a tile, as RuneLite's WorldPoint.getRegionID.
func region(p WalkParams) int { return (p.X>>6)<<8 | p.Y>>6 }

// unsafeRegions are the regions of minigames, raids and bosses, from
// RuneLite's Discord plugin (DiscordGameEventType: MINIGAMES, RAIDS and
// BOSSES), plus the sinking Fishing Trawler of its Fishing plugin.
var unsafeRegions = map[int]string{
	11851: "Abyssal Sire",
	11850: "Abyssal Sire",
	12363: "Abyssal Sire",
	12362: "Abyssal Sire",
	14489: "Araxxor",
	4883:  "Cerberus",
	5140:  "Cerberus",
	5395:  "Cerberus",
	11602: "Commander Zilyana",
	11842: "Corporeal Beast",
	11844: "Corporeal Beast",
	11588: "Dagannoth Kings",
	11589: "Dagannoth Kings",
	12132: "Duke Sucellus",
	11347: "General Graardor",
	6993:  "Giant Mole",
	6992:  "Giant Mole",
	6727:  "Grotesque Guardians",
	5021:  "Hespori",
	5536:  "Alchemical Hydra",
	13972: "Kalphite Queen",
	9116:  "Kraken",
	11346: "Kree'arra",
	11603: "K'ril Tsutsaroth",
	11601: "Nex",
	15515: "Nightmare of Ashihama",
	11330: "Phantom Muspah",
	7322:  "Sarachnis",
	9048:  "Skotizo",
	9363:  "Thermonuclear smoke devil",
	9619:  "Thermonuclear smoke devil",
	12076: "Tempoross",
	8291:  "The Leviathan",
	11669: "The Royal Titans",
	10595: "The Whisperer",
	4405:  "Vardorvis",
	9023:  "Vorkath",
	6462:  "Wintertodt",
	6045:  "Yama",
	12126: "Zalcano",
	9007:  "Zulrah",
	10646: "Ardougne Rat Pits",
	7508:  "Barbarian Assault",
	7509:  "Barbarian Assault",
	10322: "Barbarian Assault",
	14131: "Barrows",
	14231: "Barrows",
	7757:  "Blast Furnace",
	11157: "Brimhaven Agility Arena",
	8781:  "Burthorpe Games Room",
	9520:  "Castle Wars",
	9620:  "Castle Wars",
	12621: "Clan Wars",
	12622: "Clan Wars",
	12623: "Clan Wars",
	13130: "Clan Wars",
	13131: "Clan Wars",
	13133: "Clan Wars",
	13134: "Clan Wars",
	13135: "Clan Wars",
	13386: "Clan Wars",
	13387: "Clan Wars",
	13390: "Clan Wars",
	13641: "Clan Wars",
	13642: "Clan Wars",
	13643: "Clan Wars",
	13644: "Clan Wars",
	13645: "Clan Wars",
	13646: "Clan Wars",
	13647: "Clan Wars",
	13899: "Clan Wars",
	13900: "Clan Wars",
	14155: "Clan Wars",
	14156: "Clan Wars",
	13362: "Emir's Arena",
	13363: "Emir's Arena",
	7499:  "Fishing Trawler",
	7216:  "Fortis Colosseum",
	7316:  "Fortis Colosseum Lobby",
	12127: "The Gauntlet",
	7512:  "The Gauntlet",
	7768:  "Corrupted Gauntlet",
	13491: "Giants' Foundry",
	14484: "Guardians of the Rift",
	8797:  "Hallowed Sepulchre",
	9051:  "Hallowed Sepulchre",
	9052:  "Hallowed Sepulchre",
	9053:  "Hallowed Sepulchre",
	9054:  "Hallowed Sepulchre",
	9309:  "Hallowed Sepulchre",
	9563:  "Hallowed Sepulchre",
	9565:  "Hallowed Sepulchre",
	9821:  "Hallowed Sepulchre",
	10074: "Hallowed Sepulchre",
	10075: "Hallowed Sepulchre",
	10077: "Hallowed Sepulchre",
	9043:  "The Inferno",
	7753:  "Keldagrim Rat Pits",
	13658: "LMS - Deserted Island",
	13659: "LMS - Deserted Island",
	13660: "LMS - Deserted Island",
	13914: "LMS - Deserted Island",
	13915: "LMS - Deserted Island",
	13916: "LMS - Deserted Island",
	13918: "LMS - Wild Varrock",
	13919: "LMS - Wild Varrock",
	13920: "LMS - Wild Varrock",
	14174: "LMS - Wild Varrock",
	14175: "LMS - Wild Varrock",
	14176: "LMS - Wild Varrock",
	14430: "LMS - Wild Varrock",
	14431: "LMS - Wild Varrock",
	14432: "LMS - Wild Varrock",
	13462: "Mage Training Arena",
	13463: "Mage Training Arena",
	9033:  "Nightmare Zone",
	10536: "Pest Control",
	11926: "Port Sarim Rat Pits",
	7749:  "Pyramid Plunder",
	11854: "Rogues' Den",
	11855: "Rogues' Den",
	12109: "Rogues' Den",
	12110: "Rogues' Den",
	12111: "Rogues' Den",
	11605: "Sorceress's Garden",
	8493:  "Soul Wars",
	8748:  "Soul Wars",
	8749:  "Soul Wars",
	9005:  "Soul Wars",
	8014:  "Temple Trekking",
	8270:  "Temple Trekking",
	8256:  "Temple Trekking",
	8782:  "Temple Trekking",
	9038:  "Temple Trekking",
	9294:  "Temple Trekking",
	9550:  "Temple Trekking",
	9806:  "Temple Trekking",
	7222:  "Tithe Farm",
	15150: "Trouble Brewing",
	9551:  "Tzhaar Fight Caves",
	9552:  "Tzhaar Fight Pits",
	11599: "Varrock Rat Pits",
	15263: "Volcanic Mine",
	15262: "Volcanic Mine",
	12889: "Chambers of Xeric",
	13136: "Chambers of Xeric",
	13137: "Chambers of Xeric",
	13138: "Chambers of Xeric",
	13139: "Chambers of Xeric",
	13140: "Chambers of Xeric",
	13141: "Chambers of Xeric",
	13145: "Chambers of Xeric",
	13393: "Chambers of Xeric",
	13394: "Chambers of Xeric",
	13395: "Chambers of Xeric",
	13396: "Chambers of Xeric",
	13397: "Chambers of Xeric",
	13401: "Chambers of Xeric",
	12611: "Theatre of Blood",
	12612: "Theatre of Blood",
	12613: "Theatre of Blood",
	12867: "Theatre of Blood",
	12869: "Theatre of Blood",
	13122: "Theatre of Blood",
	13123: "Theatre of Blood",
	13125: "Theatre of Blood",
	13379: "Theatre of Blood",
	14160: "Tombs of Amascut",
	14162: "Tombs of Amascut",
	14164: "Tombs of Amascut",
	14674: "Tombs of Amascut",
	14676: "Tombs of Amascut",
	15184: "Tombs of Amascut",
	15186: "Tombs of Amascut",
	15188: "Tombs of Amascut",
	15696: "Tombs of Amascut",
	15698: "Tombs of Amascut",
	15700: "Tombs of Amascut",
	16214: "The Fractured Archive",
	16726: "The Fractured Archive",
	16215: "The Fractured Archive",
	16727: "The Fractured Archive",
	16216: "The Fractured Archive",
	16728: "The Fractured Archive",
	16472: "The Fractured Archive",
	16474: "The Fractured Archive",
	13454: "Jaltevas Pyramid",
	14672: "Osmumten's Burial Chamber",
	8011:  "Fishing Trawler (sinking)",
}
