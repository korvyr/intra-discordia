package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/o1egl/govatar"
	"github.com/rylio/ytdl"

	_ "github.com/lib/pq"
)

const (
	sqlDriver = "postgres"
	sqlSource = "postgresql://discord@localhost:26257/discord?sslmode=disable"
)

var (
	db *sql.DB
)

type ddgResult struct {
	Answer           string      `json:"Answer"`
	Heading          string      `json:"Heading"`
	ImageWidth       interface{} `json:"ImageWidth"`
	Entity           string      `json:"Entity"`
	Type             string      `json:"Type"`
	DefinitionSource string      `json:"DefinitionSource"`
	ImageHeight      interface{} `json:"ImageHeight"`
	Infobox          interface{} `json:"Infobox"`
	AbstractSource   string      `json:"AbstractSource"`
	AnswerType       string      `json:"AnswerType"`
	RelatedTopics    []struct {
		FirstURL string `json:"FirstURL"`
		Result   string `json:"Result"`
		Text     string `json:"Text"`
		Icon     struct {
			Width  string `json:"Width"`
			URL    string `json:"URL"`
			Height string `json:"Height"`
		} `json:"Icon"`
	} `json:"RelatedTopics"`
	Redirect string `json:"Redirect"`
	Results  []struct {
		Result   string `json:"Result"`
		FirstURL string `json:"FirstURL"`
		Text     string `json:"Text"`
		Icon     struct {
			Width  int    `json:"Width"`
			URL    string `json:"URL"`
			Height int    `json:"Height"`
		} `json:"Icon"`
	} `json:"Results"`
	Meta struct {
		JsCallbackName string      `json:"js_callback_name"`
		Blockgroup     interface{} `json:"blockgroup"`
		DevDate        interface{} `json:"dev_date"`
		SrcOptions     struct {
			SourceSkip        string `json:"source_skip"`
			IsMediawiki       int    `json:"is_mediawiki"`
			SkipEnd           string `json:"skip_end"`
			IsFanon           int    `json:"is_fanon"`
			SkipIcon          int    `json:"skip_icon"`
			Language          string `json:"language"`
			SrcInfo           string `json:"src_info"`
			SkipImageName     int    `json:"skip_image_name"`
			Directory         string `json:"directory"`
			SkipAbstract      int    `json:"skip_abstract"`
			IsWikipedia       int    `json:"is_wikipedia"`
			MinAbstractLength string `json:"min_abstract_length"`
			SkipAbstractParen int    `json:"skip_abstract_paren"`
			SkipQr            string `json:"skip_qr"`
		} `json:"src_options"`
		SrcID           int         `json:"src_id"`
		Producer        interface{} `json:"producer"`
		SrcURL          interface{} `json:"src_url"`
		IsStackexchange interface{} `json:"is_stackexchange"`
		Maintainer      struct {
			Github string `json:"github"`
		} `json:"maintainer"`
		DevMilestone    string      `json:"dev_milestone"`
		PerlModule      string      `json:"perl_module"`
		Unsafe          int         `json:"unsafe"`
		ProductionState string      `json:"production_state"`
		Status          string      `json:"status"`
		Designer        interface{} `json:"designer"`
		Tab             string      `json:"tab"`
		Repo            string      `json:"repo"`
		Attribution     interface{} `json:"attribution"`
		Topic           []string    `json:"topic"`
		Developer       []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
			Name string `json:"name"`
		} `json:"developer"`
		Name         string      `json:"name"`
		SrcName      string      `json:"src_name"`
		SignalFrom   string      `json:"signal_from"`
		SrcDomain    string      `json:"src_domain"`
		ExampleQuery string      `json:"example_query"`
		CreatedDate  interface{} `json:"created_date"`
		LiveDate     interface{} `json:"live_date"`
		ID           string      `json:"id"`
		Description  string      `json:"description"`
	} `json:"meta"`
	AbstractURL   string      `json:"AbstractURL"`
	Abstract      string      `json:"Abstract"`
	AbstractText  string      `json:"AbstractText"`
	DefinitionURL string      `json:"DefinitionURL"`
	Definition    string      `json:"Definition"`
	Image         string      `json:"Image"`
	ImageIsLogo   interface{} `json:"ImageIsLogo"`
}

func main() {
	// slurp up the token
	b, e := ioutil.ReadFile("./token")
	if e != nil {
		panic(e)
	}
	token := string(b)
	// connect to database
	db, e = sql.Open(sqlDriver, sqlSource)
	if e != nil {
		panic(e)
	}
	log.Println("Database connected...")
	// init bot
	s, e := discordgo.New("Bot " + token)
	if e != nil {
		panic(e)
	}
	// connect bot
	e = s.Open()
	if e != nil {
		panic(e)
	}
	log.Println("Bot online...")
	// get connected guilds
	gs := s.State.Guilds
	// iterate through each guild
	for _, g := range gs {

	}
	// pass session its handlers
	s.AddHandler(joinHandler)
	s.AddHandler(leaveHandler)
	s.AddHandler(msgHandler)
	log.Println("Bot listening...")
	// update status
	s.UpdateStatus(0, "| ?help")
	// keep bot alive
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}

func jukebox(s *discordgo.Session, g *discordgo.Guild) {
	cs, e := s.GuildChannels(g.ID)
	if e != nil {
		trace(e)
		return
	}
	cID := ""
	for _, c := range cs {
		if strings.ToLower(c.Name) == "jukebox" {
			cID = c.ID
		}
	}
	if cID == "" {
		trace(errors.New("no jukebox channel found"))
		return
	}
	v, e := s.ChannelVoiceJoin(g.ID, cID, false, true)
	if e == nil {
		// clean out any old connections
		v.Speaking(false)
		v.Disconnect()
		v.Close()
	}
	v, e = s.ChannelVoiceJoin(g.ID, cID, false, true)
	if e != nil {
		trace(e)
		return
	}
	stop := make(chan bool)
	var que []string
	for ctl := range jbControl {
		cmd := strings.Split(ctl, " ")
		if cmd[0] == "play" {
			if len(cmd) < 2 {
				trace(errors.New("jbControl play: missing link"))
				continue
			}
			link := cmd[1]
			select {
			case stop <- false:
				que = append(que, link)
			default:
				go func() {
					dgvoice.PlayAudioFile(v, link, stop)
					jbControl <- "done"
				}()
			}
			continue
		}
		if cmd[0] == "stop" {
			que = []string{}
			jbPlaylist = []string{}
			jbSkip = []string{}
			select {
			case stop <- true:
			default:
			}
			continue
		}
		if cmd[0] == "next" {
			select {
			case stop <- true:
			default:
			}
			continue
		}
		if cmd[0] == "reset" {
			v.Disconnect()
			v.Close()
			v, e = s.ChannelVoiceJoin(g.ID, cID, false, true)
			if e != nil {
				trace(e)
				continue
			}
			go func() {
				jbControl <- "done"
			}()
			continue
		}
		if cmd[0] == "done" {
			if runtime.GOOS == "linux" {
				kill := exec.Command("killall", "ffmpeg")
				_ = kill.Run
			}
			select {
			case stop <- true:
			default:
			}
			jbPlaylist = remFirst(jbPlaylist)
			jbSkip = []string{}
			link := ""
			if len(que) > 0 {
				link = que[0]
			}
			que = remFirst(que)
			if link == "" {
				continue
			}
			go func() {
				dgvoice.PlayAudioFile(v, link, stop)
				jbControl <- "done"
			}()
			continue
		}
	}
}

// autonomous actions
func heartbeat(s *discordgo.Session, g *discordgo.Guild) {
	tk := time.NewTicker(time.Minute * 1)
	for t := range tk.C {
		// set time to est
		now := t.In(timeZone)
		// init times
		hrago := now.Add(-time.Hour)
		hrlater := now.Add(time.Hour)
		// map roles and mentions
		role := make(map[string]string)
		rm := make(map[string]string)
		rs, e := s.GuildRoles(g.ID)
		if e != nil {
			continue
		}
		for _, r := range rs {
			role[r.Name] = r.ID
			rm[r.Name] = "<@&" + r.ID + "> "
		}
		// map channels
		channel := make(map[string]string)
		cs, e := s.GuildChannels(g.ID)
		if e != nil {
			continue
		}
		for _, c := range cs {
			channel[c.Name] = c.ID
		}

		// user message count ticker
		for k, v := range userMute {
			if v > 0 {
				userMute[k]--
			}
		}

		// clean out beeps
		go cleanBeeps(s, g)

		// clean out things voted bad
		go cleanTrash(s, g)

		// load events
		evts, e := getEvents()
		if e == nil {
			// iterate through and check times
			for id, evt := range evts {
				// skip edits
				if evt["status"] == "edit" {
					continue
				}
				// parse time
				te, e := time.ParseInLocation(timeLayout, evt["when"], timeZone)
				if e != nil {
					continue
				}
				if te.Before(hrago) {
					switch evt["often"] {
					case "once":
						sqlDeleteWithInt("events", "id", id)
						continue
					case "weekly":
						sqlUpdateWithInt("events", "datetime", te.AddDate(0, 0, 7).Format(timeLayout), "id", id)
						sqlUpdateWithInt("events", "warned", "false", "id", id)
						sqlUpdateWithInt("events", "now", "false", "id", id)
						continue
					case "monthly":
						sqlUpdateWithInt("events", "datetime", te.AddDate(0, 1, 0).Format(timeLayout), "id", id)
						sqlUpdateWithInt("events", "warned", "false", "id", id)
						sqlUpdateWithInt("events", "now", "false", "id", id)
						continue
					}
				}
				if te.Before(hrlater) && evt["warned"] != "true" {
					go s.ChannelMessageSend(channel["announcements"], rm["Officers"]+rm["NCOs"]+rm["Enlisted"]+evt["name"]+" is starting in about an hour! Prepare yourselves!")
					sqlUpdateWithInt("events", "warned", "true", "id", id)
				}
				if te.Before(now.Add(time.Minute*10)) && evt["now"] != "true" {
					go s.ChannelMessageSend(channel["announcements"], rm["Officers"]+rm["NCOs"]+rm["Enlisted"]+evt["name"]+" is starting shortly! Please make your way over to the events channels and join the server if you wish to participate!\n\nServer Info: "+evt["server"])
					sqlUpdateWithInt("events", "now", "true", "id", id)
				}
			}
		}

		// display events in schedule channel
		sch, e := s.ChannelMessages(channel["schedule"], 100, "", "", "")
		if e == nil {
			timeLast := time.Now().Add(-time.Hour * 24)
			for _, m := range sch {
				nid, nevt := nextEvent(timeLast)
				if nid == -1 {
					go s.ChannelMessageDelete(channel["schedule"], m.ID)
					continue
				}
				timeLast, e = time.ParseInLocation(timeLayout, nevt["when"], timeZone)
				if e != nil {
					continue
				}
				str := "\n..." +
					"\n\nEvent: **" + nevt["name"] + "**" +
					timesInZones(timeLast) +
					"\n\nInfo: " + nevt["info"] +
					"\n"
				go s.ChannelMessageEdit(channel["schedule"], m.ID, str)
			}
			for {
				nid, nevt := nextEvent(timeLast)
				if nid == -1 {
					break
				}
				timeLast, e = time.ParseInLocation(timeLayout, nevt["when"], timeZone)
				go s.ChannelMessageSend(channel["schedule"], "...")
			}
		}

		// iterate through voice states for xp
		for _, vs := range g.VoiceStates {
			if !vs.SelfDeaf && !vs.SelfMute && !vs.Deaf && !vs.Mute {
				mbr, e := s.GuildMember(vs.GuildID, vs.UserID)
				if e == nil {
					go updateProfile(mbr, 1)
				}
			}
		}

		// set rub's nickname
		go s.GuildMemberNickname(g.ID, "220954470446661632", "77th | Col | rubadubdub")
	}
}

// handles new members
func joinHandler(s *discordgo.Session, n *discordgo.GuildMemberAdd) {
	// get guild
	g, e := s.State.Guild(n.GuildID)
	if e != nil {
		g, e = s.Guild(n.GuildID)
		if e != nil {
			return
		}
	}
	// map channel IDs to names
	channel := make(map[string]string)
	cs, e := s.GuildChannels(g.ID)
	if e != nil {
		return
	}
	for _, c := range cs {
		channel[c.Name] = c.ID
	}
	// map role IDs to names
	role := make(map[string]string)
	rs, e := s.GuildRoles(g.ID)
	if e != nil {
		return
	}
	for _, r := range rs {
		role[r.Name] = r.ID
	}

	// build welcome list
	wc := []string{
		"Welcome Wagon",
		"Crow's Nest",
		"Brew Crew",
		"Persuade Brigade",
		"Hall Monitor",
		"Canadian",
		"Frenchiest Frenchman",
		"Baguette Inspector",
		"Croissant Taster",
		"Hon Hon Handy Man",
		"Omnipotent Oligarchy",
		"Baguette Beater",
	}
	rn := rand.Intn(len(wc))

	// welcome newcomer
	go s.ChannelMessageSend(channel["newcomers"], "Welcome to the 89e Les Enfants Perdus, "+n.User.Mention()+"! Please state your reason for visiting us today so that our staff knows how to help you.\n\nOur "+wc[rn]+" has been notified of your arrival. While you wait, please take the time to read our <#"+channel["rules"]+">.\n\nYou may type `?accept` once you're done reading the rules to accept them.")
	// DM members with notify role about newcomer
	for _, m := range g.Members {
		for _, r := range m.Roles {
			if r == role["Notify of Newcomers"] {
				chat, e := s.UserChannelCreate(m.User.ID)
				if e != nil {
					continue
				}
				go s.ChannelMessageSend(chat.ID, "A wild "+n.User.Username+" has appeared on "+g.Name+"!")
				continue
			}
		}
	}

	// apply guest role
	go s.GuildMemberRoleAdd(g.ID, n.User.ID, role["Newcomers"])

	return
}

// handles departing members
func leaveHandler(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	// get guild
	g, e := s.State.Guild(m.GuildID)
	if e != nil {
		g, e = s.Guild(m.GuildID)
		if e != nil {
			return
		}
	}
	// map channel IDs to names
	channel := make(map[string]string)
	cs, e := s.GuildChannels(g.ID)
	if e != nil {
		return
	}
	for _, c := range cs {
		channel[c.Name] = c.ID
	}
	// notify about departure
	nick := m.Nick
	if len(nick) < 1 {
		nick = m.User.Username
	}
	go s.ChannelMessageSend(channel["newcomers"], m.User.Mention()+" ("+nick+") is no longer with us...")
	return
}

// handles all messages
func msgHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// drop if bot
	if m.Author.Bot {
		return
	}

	// get channel
	ch, e := s.State.Channel(m.ChannelID)
	if e != nil {
		ch, e = s.Channel(m.ChannelID)
		if e != nil {
			return
		}
	}

	// get guild
	gld, e := s.State.Guild(ch.GuildID)
	if e != nil {
		gld, e = s.Guild(ch.GuildID)
		if e != nil {
			return
		}
	}

	// get guild member
	gldm, e := s.GuildMember(gld.ID, m.Author.ID)
	if e != nil {
		return
	}

	// get voice state channel id
	vsID := ""
	for _, v := range gld.VoiceStates {
		if v.UserID == gldm.User.ID {
			vsID = v.ChannelID
		}
	}

	// map emoji IDs to names
	emoji := make(map[string]string)
	for _, em := range gld.Emojis {
		emoji[em.Name] = em.APIName()
	}

	// map role IDs and mentions to names
	role := make(map[string]string)
	rm := make(map[string]string)
	rs, e := s.GuildRoles(gld.ID)
	if e != nil {
		return
	}
	for _, r := range rs {
		role[r.Name] = r.ID
		rm[r.Name] = "<@&" + r.ID + "> "
	}

	// init permissions
	isAdmin := false
	isAuth := false
	isNew := false

	// react to roles
	for _, r := range gldm.Roles {
		switch r {
		case role["Admin"]:
			isAdmin = true
		case role["Instructors"]:
			isAuth = true
		case role["Newcomers"]:
			isNew = true
		}
	}

	// mute check
	if userMute[m.Author.Mention()] > 0 {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		return
	}

	// update/create profile
	go updateProfile(gldm, 1)

	// translate if language attached to channel
	if !strings.HasPrefix(m.Content, "?") {
		if _, ok := chanLangs[ch.ID]; !ok {
			chanLangs[ch.ID] = make(map[string]string)
		}
		for _, lang := range chanLangs[ch.ID] {
			go translateMsg(s, m.Message, lang)
		}
	}

	// drop if not command
	if !strings.HasPrefix(m.Content, "?") {
		return
	}

	// set time
	msgTS, e := m.Timestamp.Parse()
	if e != nil {
		msgTS = time.Now().In(timeZone)
	}
	msgTime := msgTS.In(timeZone)

	// map channel IDs to names
	channel := make(map[string]string)
	cs, e := s.GuildChannels(gld.ID)
	if e != nil {
		return
	}
	for _, c := range cs {
		channel[c.Name] = c.ID
	}

	// split command and arguments
	msg := strings.Split(m.Content, " ")

	// common case
	msg[0] = strings.ToLower(msg[0])

	// basic help command
	if msg[0] == "?help" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		s.ChannelMessageSend(ch.ID, "*[beep]* Current commands:"+
			"\n      __*Everyone*__"+
			"\n**?help** - this"+
			"\n**?ask <question>** - will ask le Empereur a question"+
			"\n**?lang <language>** - will translate channel to the given language"+
			"\n**?flip** - flips a coin"+
			"\n**?roll <dice>** - rolls dice of the kind given"+
			"\n**?profile [<@s>]** - displays info on the individual(s)"+
			"\n**?top** - lists the top 10 users"+
			"\n**?me <opt> [<val>]** - set/view a profile option on yourself"+
			"\n**?ping** - bot latency"+
			"\n**?avatar [<male/female>] [<seed>]** - generates an avatar"+
			"\n**?jukebox <search/play/next/list/reset> [<song>]** - jukebox controls"+
			"\n      __*Admin / Instructors*__"+
			"\n**?enlist <@s>** - adds cadet roles and proper tags"+
			"\n**?promote <@s>** - promotes a cadet to their next rank"+
			"\n**?job <job> <@s>** - assign a SCDT to their career field with tags and roles"+
			"\n**?rep <@s>** - sets representative roles"+
			"\n**?merc <@s>** - sets mercenary roles"+
			"\n**?visitor <@s>** - sets the visitor roles"+
			"\n**?attend <event/training> [<number>] [<@s>]** - credit for attendance"+
			"\n**?title <title> <@s>** - awards a title"+
			"\n\n*(Text in <> are variables. Text in [] are optionals.)*")
		return
	}

	// WAAAAAAAAH
	if msg[0] == "?wah" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		s.ChannelMessageSend(ch.ID, "*[beep]* WA"+strings.Repeat("A", rand.Intn(30))+"AH!"+strings.Repeat("!", rand.Intn(10)))
	}

	// get the latest AGS update
	if msg[0] == "?ags" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		ds, e := s.ChannelMessages(ch.ID, 10, "", "", "")
		if e != nil {
			return
		}
		for _, d := range ds {
			go s.MessageReactionAdd(ch.ID, d.ID, emoji["waluigi"])
		}
		s.ChannelMessageSend(ch.ID, "*[beep]* WALUIGI TIME")
		return
	}

	// newcomers accept rules
	if msg[0] == "?accept" && ch.ID == channel["newcomers"] && isNew {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		s.ChannelMessageSend(ch.ID, "Thank you for taking the time to read our rules, "+m.Author.Mention()+"! We have higher standards than most, and we like to make sure our newcomers know what they are so there are no surprises.")
		return
	}

	// set profile options
	if msg[0] == "?me" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			s.ChannelMessageSend(ch.ID, "*[beep]* Current options: steam, forum, regiment, faction, region\ne.g. `?me faction French Empire`")
			return
		}
		if msg[1] != "steam" && msg[1] != "forum" && msg[1] != "regiment" && msg[1] != "faction" && msg[1] != "region" {
			s.ChannelMessageSend(ch.ID, "*[beep]* Current options: steam, forum, regiment, faction, region\ne.g. `?me faction French Empire`")
			return
		}
		if len(msg) < 3 {
			reply, e := sqlGet("users", msg[1], "mention", m.Author.Mention())
			if e == nil {
				s.ChannelMessageSend(ch.ID, "*[beep]* **"+strings.Title(msg[1])+":** "+reply)
			}
			return
		}
		e := sqlUpdate("users", msg[1], strings.Join(msg[2:], " "), "mention", m.Author.Mention())
		if e == nil {
			s.ChannelMessageSend(ch.ID, "*[beep]* "+msg[1]+" updated successfully")
		} else {
			s.ChannelMessageSend(ch.ID, "*[beep]* "+msg[1]+" update failed: `"+e.Error()+"`")
		}
		return
	}

	// list the top 10 users
	if msg[0] == "?top" {
		go s.ChannelMessageDelete(ch.ID, m.ID)

		db, e := sql.Open(sqlDriver, sqlSource)
		defer db.Close()
		if e != nil {
			trace(e)
			return
		}

		var top []string
		var prt []string
		var i int

		top = append(top, "Top Users:")
		prt = append(prt, "Top Participants:")

		q, e := db.Query("select name from users order by xp desc")
		if e != nil {
			trace(e)
		}
		i = 0
		for q.Next() {
			i++
			var v string
			e = q.Scan(&v)
			trace(e)
			if e == nil {
				top = append(top, strconv.Itoa(i)+") "+v)
			}
		}

		q, e = db.Query("select name from users order by (events+trainings) desc")
		if e != nil {
			trace(e)
		}
		i = 0
		for q.Next() {
			i++
			var v string
			e = q.Scan(&v)
			trace(e)
			if e == nil {
				prt = append(prt, strconv.Itoa(i)+") "+v)
			}
		}

		reply := "*[beep]* ```\n"
		for i := 0; i < 6; i++ {
			if len(top) > i {
				reply += padRight(top[i], " ", 40)
			} else {
				reply += padRight("", " ", 40)
			}
			if len(prt) > i {
				reply += prt[i]
			}
			reply += "\n"
		}
		reply += "```"

		s.ChannelMessageSend(ch.ID, reply)
		return
	}

	// get info on a person
	if msg[0] == "?profile" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			m.Mentions = append(m.Mentions, m.Author)
		}
		for _, v := range m.Mentions {
			t, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			if len(t.Nick) == 0 {
				t.Nick = t.User.Username
			}
			ue, e := sqlExists("users", "mention", t.User.Mention())
			if e != nil {
				continue
			}
			if !ue {
				e = sqlInsert("users", "mention", t.User.Mention())
				if e != nil {
					continue
				}
			}
			go sqlUpdate("users", "name", t.Nick, "mention", t.User.Mention())
			go sqlUpdate("users", "joined", t.JoinedAt, "mention", t.User.Mention())

			// these return empty values if failed, so no harm in ignoring their errors
			tlvl, _ := sqlGetInt("users", "level", "mention", t.User.Mention())
			txp, _ := sqlGetInt("users", "xp", "mention", t.User.Mention())
			tlvlpct := txp % xpPerLvl * 100 / xpPerLvl
			treg, _ := sqlGet("users", "regiment", "mention", t.User.Mention())
			tfac, _ := sqlGet("users", "faction", "mention", t.User.Mention())
			tsteam, _ := sqlGet("users", "steam", "mention", t.User.Mention())
			tforum, _ := sqlGet("users", "forum", "mention", t.User.Mention())
			trgn, _ := sqlGet("users", "region", "mention", t.User.Mention())
			tevts, _ := sqlGetInt("users", "events", "mention", t.User.Mention())
			ttngs, _ := sqlGetInt("users", "trainings", "mention", t.User.Mention())
			tle, _ := sqlGet("users", "lastevent", "mention", t.User.Mention())
			tlt, _ := sqlGet("users", "lasttraining", "mention", t.User.Mention())

			reply := "*[beep]* Current dossier:"
			reply += "\n**Name:** `" + t.Nick + "`"
			if len(treg) > 0 {
				reply += "  **Regiment:** `" + treg + "`"
			}
			reply += "\n**Level:** `" + strconv.Itoa(tlvl) + " (" + strconv.Itoa(tlvlpct) + "%)`"
			if len(tfac) > 0 {
				reply += strings.Repeat(" ", len(t.Nick)+(len(t.Nick)/2)) + "  **Faction:** `" + tfac + "`"
			}
			reply += "\n**Events:** `" + strconv.Itoa(tevts) + " (Last: " + tle[:10] + ")`"
			reply += "  **Trainings:** `" + strconv.Itoa(ttngs) + " (Last: " + tlt[:10] + ")`"
			reply += "\n**Joined:** `" + t.JoinedAt[:10] + "`"
			if len(trgn) > 0 {
				reply += "\n**Region:** `" + trgn + "`"
			}
			if len(tsteam) > 0 {
				reply += "\n**Steam Profile:** " + tsteam
			}
			if len(tforum) > 0 {
				reply += "\n**Forum Profile:** " + tforum
			}
			reply += "\n**Avatar:** " + t.User.AvatarURL("256")

			go s.ChannelMessageSend(ch.ID, reply)
		}
		return
	}

	// ask a question
	if msg[0] == "?ask" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			go s.ChannelMessageDelete(ch.ID, m.ID)
			s.ChannelMessageSend(ch.ID, "*[beep]* What do you want to know?")
			return
		}
		r, e := ddgSearch(strings.Join(msg[1:], " "))
		if e != nil {
			trace(e)
			return
		}
		a := "*[beep]* "
		if len(r.Heading) > 0 {
			a += "**" + r.Heading + "**\n"
		}
		if len(r.Answer) > 0 {
			a += r.Answer + "\n"
			a += r.AbstractURL + "\n"
		}
		if len(r.Definition) > 0 {
			a += r.Definition + "\n"
			a += r.DefinitionURL + "\n"
		}
		if len(r.AbstractText) > 0 {
			a += r.AbstractText + "\n"
			a += r.AbstractURL + "\n"
		}
		if len(r.Results) > 0 {
			a += r.Results[0].Text + "\n"
		}
		if len(r.Redirect) > 0 {
			a += r.Redirect + "\n"
		}
		if len(r.Image) > 0 {
			a += r.Image + "\n"
		}
		if len(r.RelatedTopics) > 0 {
			a += r.RelatedTopics[0].Text + "\n"
		}
		if a == "*[beep]* " {
			switch rand.Intn(3) {
			case 0:
				a += "***Oui***"
			case 1:
				a += "***Non***"
			case 2:
				a += "***Peut-être***"
			}
		}
		if len(a) > 1999 {
			s.ChannelMessageSend(ch.ID, a[:1999])
		} else {
			s.ChannelMessageSend(ch.ID, a)
		}
		return
	}

	// roll a die
	if msg[0] == "?roll" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			s.ChannelMessageSend(ch.ID, "*[beep]* Please specify die or dice to roll. e.g. `?roll 2d6 1d4`")
			return
		}
		var res string
		var cnt int
		for _, die := range msg[1:] {
			x := strings.Split(die, "d")
			if len(x) < 2 {
				continue
			}
			num, e := strconv.Atoi(x[0])
			if e != nil || num <= 0 {
				num = 1
			}
			if num > 20 {
				num = 20
			}
			sds, e := strconv.Atoi(x[1])
			if e != nil || sds <= 2 || sds > 20 {
				sds = 6
			}
			for i := 0; i < num; i++ {
				roll := rand.Intn(sds) + 1
				res += "d" + strconv.Itoa(sds) + "(**" + strconv.Itoa(roll) + "**) "
				cnt += roll
			}
		}
		s.ChannelMessageSend(ch.ID, "*[beep]* Results: "+res+"= **"+strconv.Itoa(cnt)+"**")
		return
	}

	// flip a coin
	if msg[0] == "?flip" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		x := rand.Intn(2)
		var flip string
		if x == 0 {
			flip = "HEADS"
		} else {
			flip = "TAILS"
		}
		s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" flips a coin: ***"+flip+"***")
		return
	}

	// quick alive test
	if msg[0] == "?ping" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		s.ChannelMessageSend(m.ChannelID, "*[beep]* Pong: `"+strconv.Itoa(int(time.Since(msgTime).Nanoseconds())/1000000)+"ms`")
		return
	}

	if msg[0] == "?event" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		_, evt := nextEvent(time.Now().In(timeZone).Add(-time.Hour))
		evtTime, e := time.ParseInLocation(timeLayout, evt["when"], timeZone)
		if e != nil {
			return
		}
		s.ChannelMessageSend(ch.ID, "*[beep]* "+
			"\n\nEvent: **"+evt["name"]+"**"+
			timesInZones(evtTime)+
			"\n\nInfo: "+evt["info"]+
			"\n")
		return
	}

	// enable/disable translations on channel
	if msg[0] == "?lang" || msg[0] == "language" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			m := "*[beep]* Please provide a language to add or remove from this channel.\n"
			if _, ok := chanLangs[ch.ID]; ok && len(chanLangs[ch.ID]) > 0 {
				m += "Current languages on this channel: "
				for k := range chanLangs[ch.ID] {
					m += k + " "
				}
			} else {
				m += "No languages set on this channel."
			}
			s.ChannelMessageSend(ch.ID, m)
			return
		}
		lang := strings.Title(msg[1])
		if lang == "None" {
			chanLangs[ch.ID] = make(map[string]string)
			s.ChannelMessageSend(ch.ID, "*[beep]* Removed all languages from channel.")
			return
		}
		if _, ok := languages[lang]; !ok {
			s.ChannelMessageSend(ch.ID, "*[beep]* "+lang+" is not a valid language.")
			return
		}
		if _, ok := chanLangs[ch.ID]; !ok {
			chanLangs[ch.ID] = make(map[string]string)
		}
		if _, ok := chanLangs[ch.ID][lang]; !ok {
			chanLangs[ch.ID][lang] = languages[lang]
			s.ChannelMessageSend(ch.ID, "*[beep]* "+lang+" added to channel.")
		} else {
			delete(chanLangs[ch.ID], lang)
			s.ChannelMessageSend(ch.ID, "*[beep]* "+lang+" removed from channel.")
		}
		return
	}

	// jukebox
	if msg[0] == "?jb" || msg[0] == "?jukebox" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if vsID != channel["Jukebox"] {
			s.ChannelMessageSend(ch.ID, "*[beep]* You must be in the Jukebox voice channel to control it.")
			return
		}
		if len(msg) < 2 {
			s.ChannelMessageSend(ch.ID, "*[beep]* Jukebox what? `?jb <play/skip/list> [<song>]` e.g. `?jb play boys are back in town` or `?jb skip` to vote to skip")
			return
		}
		msg[1] = strings.ToLower(msg[1])
		if msg[1] == "play" || msg[1] == "add" {
			if len(msg) < 3 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Play what? `?jb <play/skip/list> [<song>]` e.g. `?jb play boys are back in town` or `?jb skip` to vote to skip")
				return
			}
			vIDs, e := ytSearch(strings.Join(msg[2:], " "))
			if e != nil {
				trace(e)
				return
			}
			if len(vIDs) == 0 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Song not found. (YouTube API)")
				return
			}
			v, e := ytdl.GetVideoInfoFromID(vIDs[0])
			if e != nil {
				trace(e)
				return
			}
			f := v.Formats.Extremes(ytdl.FormatAudioBitrateKey, true)[0]
			u, e := v.GetDownloadURL(f)
			if e != nil {
				trace(e)
				return
			}
			if v.Duration > time.Minute*10 {
				s.ChannelMessageSend(ch.ID, "*[beep]* I'm sorry, that song is too long. (>10m)")
				return
			}
			jbControl <- "play " + u.String()
			if len(gldm.Nick) == 0 {
				gldm.Nick = gldm.User.Username
			}
			jbPlaylist = append(jbPlaylist, padRight(gldm.Nick, " ", 25)+" | "+v.Title+" ("+v.Duration.String()+")")
			s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" added `"+v.Title+" ("+v.Duration.String()+")` to the Jukebox.")
			return
		}
		if msg[1] == "stop" {
			if isAdmin {
				jbControl <- "stop"
				s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" silenced the Jukebox.")
			}
			return
		}
		if msg[1] == "next" || msg[1] == "skip" {
			var jbUsers int
			for _, v := range gld.VoiceStates {
				if v.ChannelID == channel["Jukebox"] {
					jbUsers++
				}
			}
			for _, i := range jbSkip {
				if i == m.Author.Mention() {
					jbSkip = remFirst(jbSkip)
				}
			}
			jbSkip = append(jbSkip, m.Author.Mention())
			s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" voted to skip this song. ("+strconv.Itoa(len(jbSkip))+"/"+strconv.Itoa(jbUsers/2)+")")
			if len(jbSkip) >= jbUsers/2 {
				go s.ChannelMessageSend(ch.ID, "*[beep]* Skipping current song with majority vote.")
				jbControl <- "next"
			}
			return
		}
		if msg[1] == "reset" || msg[1] == "kick" {
			if isAdmin {
				jbControl <- "reset"
				s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" has given the Jukebox a kick.")
			}
			return
		}
		if msg[1] == "search" {
			if len(msg) < 3 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Search for what? e.g. `?jb search boys are back in town`")
				return
			}
			vIDs, e := ytSearch(strings.Join(msg[2:], " "))
			if e != nil {
				trace(e)
				return
			}
			jbSearch = []ytdl.VideoInfo{}
			list := "*[beep]* Your search found:\n```\n"
			for i, vID := range vIDs {
				vidInfo, e := ytdl.GetVideoInfoFromID(vID)
				if e != nil {
					trace(e)
					return
				}
				jbSearch = append(jbSearch, *vidInfo)
				list += "[" + strconv.Itoa(i) + "] " + vidInfo.Title + " (" + vidInfo.Duration.String() + ")\n"
			}
			list += "```\nUse `?jb select <num>` to pick a song from this list."
			s.ChannelMessageSend(ch.ID, list)
			return
		}
		if msg[1] == "select" || msg[1] == "sel" {
			if len(jbSearch) == 0 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Search results found. `?jb search <song>`")
				return
			}
			if len(msg) < 3 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Select what? e.g. `?jb select 0`")
				return
			}
			sel, e := strconv.Atoi(msg[2])
			if e != nil {
				s.ChannelMessageSend(ch.ID, "*[beep]* Select what? `?jb select <num>`")
				return
			}
			if sel < 0 || sel > len(jbSearch)-1 {
				s.ChannelMessageSend(ch.ID, "*[beep]* Selection not in search results.")
				return
			}
			v := jbSearch[sel]
			f := v.Formats.Extremes(ytdl.FormatAudioBitrateKey, true)[0]
			u, e := v.GetDownloadURL(f)
			if e != nil {
				trace(e)
				return
			}
			if v.Duration > time.Minute*10 {
				s.ChannelMessageSend(ch.ID, "*[beep]* I'm sorry, that song is too long. (>10m)")
				return
			}
			jbControl <- "play " + u.String()
			if len(gldm.Nick) == 0 {
				gldm.Nick = gldm.User.Username
			}
			jbPlaylist = append(jbPlaylist, padRight(gldm.Nick, " ", 25)+" | "+v.Title+" ("+v.Duration.String()+")")
			s.ChannelMessageSend(ch.ID, "*[beep]* "+m.Author.Mention()+" added `"+v.Title+" ("+v.Duration.String()+")` to the Jukebox.")
			return
		}
		if msg[1] == "list" || msg[1] == "playlist" {
			list := ""
			for i, t := range jbPlaylist {
				list += "[" + padLeft(strconv.Itoa(i), "0", 2) + "] " + t + "\n"
			}
			s.ChannelMessageSend(ch.ID, "*[beep]* Current jukebox playlist:\n```\n"+list+"\n```")
			return
		}
		return
	}

	if msg[0] == "?avatar" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		genders := []govatar.Gender{govatar.MALE, govatar.FEMALE}
		gender := genders[rand.Intn(2)]
		content := "random"
		if len(msg) > 1 {
			if strings.ToLower(msg[1]) == "male" {
				gender = govatar.MALE
				content = "male"
			} else if strings.ToLower(msg[1]) == "female" {
				gender = govatar.FEMALE
				content = "female"
			}
		}
		var avatar io.Reader
		if len(msg) > 2 {
			avatarImage, e := govatar.GenerateFromUsername(gender, msg[2])
			if e != nil {
				trace(e)
				return
			}
			var buf bytes.Buffer
			png.Encode(&buf, avatarImage)
			avatar = bytes.NewReader(buf.Bytes())
			content += "/" + msg[2]
		} else {
			avatarImage, e := govatar.Generate(gender)
			if e != nil {
				trace(e)
				return
			}
			var buf bytes.Buffer
			png.Encode(&buf, avatarImage)
			avatar = bytes.NewReader(buf.Bytes())
			content += "/random"
		}
		s.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
			Content: "*[beep]* Avatar Generated (" + content + "):",
			File: &discordgo.File{
				Name:   "avatar.png",
				Reader: avatar,
			},
			Embed: &discordgo.MessageEmbed{
				Image: &discordgo.MessageEmbedImage{
					URL: "attachment://avatar.png",
				},
			},
		})
		return
	}

	// authorized check
	if !isAuth && !isAdmin {
		return
	}

	// attendance keeper
	if msg[0] == "?attend" || msg[0] == "?attended" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			s.ChannelMessageSend(ch.ID, "*[beep]* `?attend <event/training> [<int>] [<@s>]` e.g. `?attend event @jimbo @bobbyboi`")
			return
		}
		if msg[1] != "event" && msg[1] != "training" {
			s.ChannelMessageSend(ch.ID, "*[beep]* `?attend <event/training> [<int>] [<@s>]` e.g. `?attend event @jimbo @bobbyboi`")
			return
		}
		cnt := 1
		if len(msg) > 2 {
			cnt, e = strconv.Atoi(msg[2])
			if e != nil {
				cnt = 1
			}
		}
		if len(m.Mentions) == 0 {
			for _, v := range gld.VoiceStates {
				c, e := s.Channel(v.ChannelID)
				if e != nil {
					continue
				}
				if strings.HasPrefix(strings.ToLower(c.Name), "event") {
					mbr, e := s.GuildMember(gld.ID, v.UserID)
					if e != nil {
						continue
					}
					ev, e := sqlGetInt("users", msg[1]+"s", "mention", mbr.User.Mention())
					if e != nil {
						continue
					}
					e = sqlUpdateInt("users", msg[1]+"s", ev+cnt, "mention", mbr.User.Mention())
					if e != nil {
						continue
					}
					go updateProfile(mbr, 100*cnt)
					go sqlUpdateToday("users", "last"+msg[1], "mention", mbr.User.Mention())
					go s.ChannelMessageSend(ch.ID, "*[beep]* "+strings.Title(msg[1])+" attendance updated for "+mbr.User.Mention()+". ("+msg[1]+"+"+strconv.Itoa(cnt)+")")
				}
			}
			return
		}
		for _, v := range m.Mentions {
			mbr, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			ev, e := sqlGetInt("users", msg[1]+"s", "mention", mbr.User.Mention())
			if e != nil {
				continue
			}
			e = sqlUpdateInt("users", msg[1]+"s", ev+cnt, "mention", mbr.User.Mention())
			if e != nil {
				continue
			}
			go updateProfile(mbr, 100*cnt)
			go sqlUpdateToday("users", "last"+msg[1], "mention", mbr.User.Mention())
			go s.ChannelMessageSend(ch.ID, "*[beep]* "+strings.Title(msg[1])+" attendance updated for "+mbr.User.Mention()+". ("+msg[1]+"+"+strconv.Itoa(cnt)+")")
		}
		return
	}

	// fake demotion
	if msg[0] == "?demote" {
		go s.ChannelMessageDelete(m.ChannelID, m.ID)
		if len(m.Mentions) < 1 {
			return
		}

		nt := []string{
			"Executive Delivery Boy",
			"1st Secretary Under Reedrick",
			"Junior Assistant to the Major",
			"Recruitment Officer",
			"Keeper of LePoof's Beads",
			"Colonel Mustache's Mustache Groomer",
			"Senior Bootlicker",
			"Supreme Pigdog Commander",
			"Señor Fluffy",
			"Genital Thunderstorm",
			"Cake's Mall-Walking Partner",
			"Cake's Cat Pooper Scooper",
			"Major Korvyr's Chair Warmer",
			"Lieutenant WarningPuzzle's Navigator",
			"Colonel Jean-Baptiste's Scheduler",
			"Reedrick's Whipping Boy",
			"Captain Mazz' Vocal Cord Fluffer",
			"Nr.9 Soldat Noble",
			"LePoof's Pistol Polisher",
		}
		ntid := rand.Intn(len(nt))

		for _, v := range m.Mentions {
			go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+m.Author.Mention()+" has demoted "+v.Mention()+" to **"+nt[ntid]+"**!")
		}
	}

	// smite
	if msg[0] == "?smite" {
		s.ChannelMessageDelete(m.ChannelID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* "+m.Author.Mention()+" calls down the **lightning** and brings down the **HAMMAH**!!!")
			return
		}
		sm := []string{
			m.Author.Mention() + " calls down **lightning** from the skies to a **giant warhammer** in their hand, bringing it ***crashing*** down on #VIC#'s head!!! *Splat...*",
			m.Author.Mention() + " raises a hand high as his palm erupts with ***conjured flames***, flinging a ball of **pure fire** at #VIC#'s face!!! *Face melting follows...*",
			m.Author.Mention() + " reaches for the heavens, *grasping* a mighty ***bolt of lightning*** in their fist. Looking down, they hurl the bolt at #VIC#!!! An ***ERUPTION*** follows, and only a crater of ash remains...",
		}
		smid := rand.Intn(len(sm))
		for _, v := range m.Mentions {
			go s.ChannelMessageSend(m.ChannelID, strings.Replace(sm[smid], "#VIC#", v.Mention(), -1))
		}
	}

	// enlist new members
	if msg[0] == "?enlist" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Who do you wish to enlist?")
			return
		}
		for _, v := range m.Mentions {
			g, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			nick := ""
			nick = g.Nick
			if len(nick) < 1 {
				nick = g.User.Username
			}
			if strings.HasPrefix(nick, "[89e]") {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is already serving with us!")
				continue
			}
			go s.GuildMemberRoleRemove(gld.ID, v.ID, role["Newcomers"])
			nick = "[89e] JCDT " + nick
			if len(nick) > 32 {
				nick = nick[:32]
			}
			go s.GuildMemberNickname(gld.ID, v.ID, nick)
			go s.GuildMemberRoleAdd(gld.ID, v.ID, role["Cadets"])
			go s.GuildMemberRoleAdd(gld.ID, v.ID, role["Enlisted"])
			go s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has enlisted "+v.Mention()+"! Welcome to the 89e! Please read over <#"+channel["recruit-handbook"]+"> as soon as you get the chance!")
		}
		return
	}

	// set a rep
	if msg[0] == "?rep" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Who do you wish to make a representative of?")
			return
		}
		for _, v := range m.Mentions {
			go s.GuildMemberRoleRemove(gld.ID, v.ID, role["Newcomers"])
			go s.GuildMemberRoleAdd(gld.ID, v.ID, role["REP"])
			go s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has given "+v.Mention()+" the representative role! Welcome to the club!")
		}
		return
	}

	// set a merc
	if msg[0] == "?merc" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Who do you wish to make a mercenary of?")
			return
		}
		for _, v := range m.Mentions {
			s.GuildMemberRoleRemove(gld.ID, v.ID, role["Newcomers"])
			s.GuildMemberRoleAdd(gld.ID, v.ID, role["Mercenaries"])
			s.GuildMemberRoleAdd(gld.ID, v.ID, role["Enlisted"])
			s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has made "+v.Mention()+" a mercenary! May you earn your gold well!")
		}
		return
	}

	// set a merc
	if msg[0] == "?visitor" || msg[0] == "?visit" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Who do you wish to make a visitor of?")
			return
		}
		for _, v := range m.Mentions {
			s.GuildMemberRoleRemove(gld.ID, v.ID, role["Newcomers"])
			s.GuildMemberRoleAdd(gld.ID, v.ID, role["Visitor"])
			s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has identified "+v.Mention()+" as a visitor! Please enjoy your stay!")
		}
		return
	}

	// promote a cadet
	if msg[0] == "?promote" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Which cadet would you like to promote? `?promote <mentions>`")
			return
		}

		grade := make(map[string]int)
		grade["JCDT"] = 0
		grade["CDT"] = 1
		grade["SCDT"] = 2

		rank := make(map[int]string)
		rank[0] = "JCDT"
		rank[1] = "CDT"
		rank[2] = "SCDT"

		for _, v := range m.Mentions {
			g, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			nick := ""
			nick = g.Nick
			if len(nick) < 1 {
				nick = g.User.Username
			}
			an := strings.Split(nick, " ")
			if an[0] != "[89e]" {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not wearing 89e tags!")
				continue
			}
			_, ok := grade[an[1]]
			if !ok {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not a cadet!")
				continue
			}
			if grade[an[1]] == 2 {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is already a SCDT! Use `?job <job> <mention>` to place in a corps.")
				continue
			}

			nr := rank[grade[an[1]]+1]
			n := "[89e] " + nr + " " + strings.Join(an[2:], " ")
			if len(n) > 32 {
				n = n[:32]
			}
			go s.GuildMemberNickname(gld.ID, v.ID, n)
			go s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has promoted "+v.Mention()+" to "+nr+"!")
		}
		return
	}

	// assign a title
	if msg[0] == "?title" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 3 || len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* What title are you giving and who should get it? `?title <title> <@s>` e.g. `?title FART @x3n0`")
			return
		}

		msg[1] = strings.ToUpper(msg[1])

		for _, v := range m.Mentions {
			g, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			nick := ""
			nick = g.Nick
			if len(nick) < 1 {
				nick = g.User.Username
			}
			an := strings.Split(nick, " ")
			if an[0] != "[89e]" {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not wearing 89e tags!")
				continue
			}
			isEnl := false
			for _, r := range g.Roles {
				if r == role["Enlisted"] {
					isEnl = true
				}
			}
			if !isEnl {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not an Enlisted member!")
				continue
			}

			go s.GuildMemberRoleAdd(gld.ID, v.ID, role["Titled"])

			n := "[89e] " + msg[1] + " " + strings.Join(an[2:], " ")
			if len(n) > 32 {
				n = n[:32]
			}
			go s.GuildMemberNickname(gld.ID, v.ID, n)
			go s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has awarded "+v.Mention()+" the title of "+msg[1]+"!")
		}
		return
	}

	// assign a job
	if msg[0] == "?job" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 3 || len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* What job are you giving and who should get it? `?job <job> <mentions>`")
			return
		}

		msg[1] = strings.ToUpper(msg[1])

		for _, v := range m.Mentions {
			g, e := s.GuildMember(gld.ID, v.ID)
			if e != nil {
				continue
			}
			nick := ""
			nick = g.Nick
			if len(nick) < 1 {
				nick = g.User.Username
			}
			an := strings.Split(nick, " ")
			if an[0] != "[89e]" {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not wearing 89e tags!")
				continue
			}
			if an[1] != "SCDT" {
				go s.ChannelMessageSend(m.ChannelID, "*[beep]* "+v.Mention()+" is not a SCDT!")
				continue
			}

			role["INF"] = role["Infantry Corps"]
			role["AUX"] = role["Auxiliary Corps"]
			role["RFLM"] = role["Rifle Corps"]
			role["ARTY"] = role["Artillery Corps"]
			role["MARINE"] = role["Marine Corps"]
			role["SN"] = role["Naval Corps"]

			_, ok := role[msg[1]]
			if !ok {
				s.ChannelMessageSend(m.ChannelID, "*[beep]* Not a valid job. `?job <job> <mentions>`")
				return
			}

			go s.GuildMemberRoleRemove(gld.ID, v.ID, role["Cadets"])
			go s.GuildMemberRoleAdd(gld.ID, v.ID, role[msg[1]])

			n := "[89e] " + msg[1] + " " + strings.Join(an[2:], " ")
			if len(n) > 32 {
				n = n[:32]
			}
			go s.GuildMemberNickname(gld.ID, v.ID, n)
			go s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+" has promoted "+v.Mention()+" to the "+msg[1]+" CORPS!")
		}
		return
	}

	// mute/unmute someone
	if msg[0] == "?mute" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(m.ChannelID, "*[beep]* Who do you wish to mute/unmute? i.g. `?mute 999 @rub`")
			return
		}
		for _, v := range m.Mentions {
			if userMute[v.Mention()] <= 0 {
				userMute[v.Mention()], e = strconv.Atoi(msg[1])
				if e != nil {
					userMute[v.Mention()] = 5
				}
				go s.ChannelMessageSend(ch.ID, v.Mention()+" has been muted for "+strconv.Itoa(userMute[v.Mention()])+" minutes. Please wait.")
				go s.ChannelMessageSend(channel["admin"], m.Author.String()+" has used mute in "+ch.Name+" on "+v.String()+" for "+strconv.Itoa(userMute[v.Mention()])+" minutes")
			} else {
				userMute[v.Mention()] = 0
				go s.ChannelMessageSend(ch.ID, "*[beep]* "+v.Mention()+" has been unmuted.")
				go s.ChannelMessageSend(channel["admin"], m.Author.String()+" has used unmute in "+ch.Name+" on "+v.String())
			}
		}
		return
	}

	// cleanup bot traffic or player traffic
	if msg[0] == "?clean" {
		go s.ChannelMessageDelete(m.ChannelID, m.ID)
		if len(m.Mentions) < 1 {
			rmsgs, e := s.ChannelMessages(m.ChannelID, 100, "", "", "")
			if e != nil {
				return
			}
			go cleanMsgs(s, rmsgs, "")
			go s.ChannelMessageSend(channel["admin"], m.Author.String()+" has used clean in "+ch.Name+" on bot and command traffic.")
			return
		}
		for _, usr := range m.Mentions {
			rmsgs, e := s.ChannelMessages(m.ChannelID, 100, "", "", "")
			if e != nil {
				continue
			}
			go cleanMsgs(s, rmsgs, usr.ID)
			go s.ChannelMessageSend(channel["admin"], m.Author.String()+" has used clean in "+ch.Name+" on "+usr.String())
		}
		return
	}

	// admin check
	if !isAdmin {
		return
	}

	// make the bot speak
	if msg[0] == "?say" {
		s.ChannelMessageDelete(m.ChannelID, m.ID)
		s.ChannelMessageSend(m.ChannelID, strings.Join(msg[1:], " "))
		return
	}

	// count or kick inactive members
	if msg[0] == "?prune" {
		if len(msg) < 2 {
			return
		}
		days, e := strconv.Atoi(msg[1])
		if e != nil {
			return
		}
		switch msg[2] {
		case "count":
			num, e := s.GuildPruneCount(gld.ID, uint32(days))
			if e != nil {
				return
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprint("*[beep]* Counting people with no activity for ", days, " days: ", num))
		case "kick":
			num, e := s.GuildPrune(gld.ID, uint32(days))
			if e != nil {
				return
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprint("*[beep]* Kicking people with no activity for ", days, " days. Count: ", num))
		}
		return
	}

	// modify events
	if msg[0] == "?schedule" || msg[0] == "?sch" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		// init edit id
		edit := -1
		// load events
		evts, e := getEvents()
		if e != nil {
			trace(e)
			return
		}
		// check for edit mode
		for id := range evts {
			if evts[id]["status"] == "edit" {
				edit = id
			}
		}
		if edit > -1 { // do edit mode if edit id set
			// not enough args, so showing current
			if len(msg) < 2 {
				s.ChannelMessageSend(m.ChannelID, "*[beep]* **SCHEDULE EDIT MODE**"+
					"\nID: "+strconv.Itoa(edit)+
					"\nname: "+evts[edit]["name"]+
					"\nwhen: "+evts[edit]["when"]+
					"\noften: "+evts[edit]["often"]+
					"\ninfo: "+evts[edit]["info"]+
					"\nserver: "+evts[edit]["server"]+
					"\n"+
					"\n`?schedule set <key> <value>`"+
					"\n`?schedule save`"+
					"\n`?schedule delete`"+
					"\n")
				return
			}
			// handle args
			switch msg[1] {
			case "set":
				if len(msg) < 4 {
					s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule set <key> <value>`")
					return
				}
				switch msg[2] {
				case "when":
					when := strings.Join(msg[3:], " ")
					if len(when) != 14 {
						s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule set when <DDMMMYYYY HHMM>` (in EST)")
						return
					}
					te, e := time.ParseInLocation(timeLayout, when, timeZone)
					if e != nil {
						s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule set when <DDMMMYYYY HHMM>` (in EST)")
						return
					}
					sqlUpdateWithInt("events", "datetime", te.Format(timeLayout), "id", edit)
				case "often":
					if msg[3] != "once" && msg[3] != "weekly" && msg[3] != "monthly" {
						s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule set often <once/weekly/monthly>`")
						return
					}
					sqlUpdateWithInt("events", "often", msg[3], "id", edit)
				case "name":
					sqlUpdateWithInt("events", "name", strings.Join(msg[3:], " "), "id", edit)
				case "info":
					sqlUpdateWithInt("events", "info", strings.Join(msg[3:], " "), "id", edit)
				case "server":
					sqlUpdateWithInt("events", "server", strings.Join(msg[3:], " "), "id", edit)
				default:
					s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule set <key> <value>`")
				}
			case "save":
				sqlUpdateWithInt("events", "status", "saved", "id", edit)
			case "delete":
				sqlDeleteWithInt("events", "id", edit)
			}
		} else { // do not edit mode if edit id not set
			// not enough args
			if len(msg) < 2 {
				s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule <list/add/edit> <id>`")
				return
			}
			// handle args
			switch msg[1] {
			case "list":
				list := ""
				for id := range evts {
					evt := evts[id]
					list += "\n" + padLeft(strconv.Itoa(id), " ", 4) + " | " + padRight(evt["name"], " ", 20) + " | " + evt["when"]
				}
				s.ChannelMessageSend(m.ChannelID, "*[beep]* Events: ```"+list+"\n```")
			case "add":
				ids, _ := sqlGetIntList("events", "id")
				id := len(ids)
				idUsed, e := sqlExistsInt("events", "id", id)
				if e != nil || idUsed {
					return
				}
				sqlInsertInt("events", "id", id)
				sqlUpdateWithInt("events", "status", "edit", "id", id)
				sqlUpdateWithInt("events", "datetime", timeLayout, "id", id)
				sqlUpdateWithInt("events", "often", "weekly", "id", id)
			case "edit":
				if len(msg) < 3 {
					s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule edit <id>`")
					return
				}
				eid, e := strconv.Atoi(msg[2])
				if e != nil {
					s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule edit <id>`")
					return
				}
				sqlUpdateWithInt("events", "status", "edit", "id", eid)
			default:
				s.ChannelMessageSend(m.ChannelID, "*[beep]* `?schedule <list/add/edit> <id>`")
			}
		}
		return
	}

	// add/remove xp
	if msg[0] == "?xp" {
		go s.ChannelMessageDelete(ch.ID, m.ID)
		if len(msg) < 2 {
			s.ChannelMessageSend(ch.ID, "*[beep]* `?xp <xp> <@s>`")
			return
		}
		xp, e := strconv.Atoi(msg[1])
		if e != nil {
			s.ChannelMessageSend(ch.ID, "*[beep]* `?xp <xp> <@s>`")
			return
		}
		if len(m.Mentions) < 1 {
			s.ChannelMessageSend(ch.ID, "*[beep]* `?xp <xp> <@s>`")
			return
		}
		for _, u := range m.Mentions {
			mbr, e := s.GuildMember(gld.ID, u.ID)
			if e != nil {
				continue
			}
			if u.ID == m.Author.ID {
				go updateProfile(mbr, -100)
				continue
			} else {
				go updateProfile(mbr, xp)
			}
			if xp < 0 {
				go s.ChannelMessageSend(ch.ID, m.Author.Mention()+" has penalized "+u.Mention()+" with "+strconv.Itoa(xp)+"xp!")
			} else {
				go s.ChannelMessageSend(ch.ID, m.Author.Mention()+" has awarded "+u.Mention()+" with "+strconv.Itoa(xp)+"xp!")
			}
		}
		return
	}
}

func isNew(key string, val string) bool {
	ex, e := sqlExists("last", "key", key)
	if e != nil {
		return false
	}
	if !ex {
		e = sqlInsert("last", "key", key)
		if e != nil {
			return false
		}
	}
	last, e := sqlGet("last", "val", "key", key)
	if e != nil {
		return false
	}
	if val != last {
		e = sqlUpdate("last", "val", val, "key", key)
		if e != nil {
			return false
		}
		return true
	}
	return false
}

func cleanMsgs(s *discordgo.Session, msgs []*discordgo.Message, id string) {
	var msgIDs []string
	if id == "" {
		for _, msg := range msgs {
			if strings.HasPrefix(msg.Content, "?") || msg.Author.Bot {
				msgIDs = append(msgIDs, msg.ID)
			}
		}
	} else {
		for _, msg := range msgs {
			if msg.Author.ID == id {
				msgIDs = append(msgIDs, msg.ID)
			}
		}
	}
	go s.ChannelMessagesBulkDelete(msgs[0].ChannelID, msgIDs)
}

// get the next event
func nextEvent(then time.Time) (int, map[string]string) {
	now := time.Now().In(timeZone)
	lt := then.In(timeZone)
	// grab events
	evts, e := getEvents()
	if e != nil {
		return -1, make(map[string]string)
	}
	// init next event id & time
	nid := -1
	nt := now.AddDate(0, 6, 0)
	// hunt for next event
	for cid, evt := range evts {
		if evt["status"] == "edit" {
			continue
		}
		ct, e := time.ParseInLocation(timeLayout, evt["when"], timeZone)
		if e != nil {
			continue
		}
		if ct.Before(nt) && ct.After(lt) {
			nt = ct
			nid = cid
		}
	}
	if nid == -1 {
		return nid, make(map[string]string)
	}
	return nid, evts[nid]
}

// load events from file
func getEvents() (map[int]map[string]string, error) {
	// init external map
	evts := make(map[int]map[string]string)
	ids, e := sqlGetIntList("events", "id")
	if e != nil {
		return evts, e
	}
	for _, id := range ids {
		evts[id] = make(map[string]string)
		evts[id]["name"], e = sqlGetWithInt("events", "name", "id", id)
		evts[id]["when"], e = sqlGetWithInt("events", "datetime", "id", id)
		evts[id]["often"], e = sqlGetWithInt("events", "often", "id", id)
		evts[id]["info"], e = sqlGetWithInt("events", "info", "id", id)
		evts[id]["server"], e = sqlGetWithInt("events", "server", "id", id)
		evts[id]["status"], e = sqlGetWithInt("events", "status", "id", id)
		evts[id]["warned"], e = sqlGetWithInt("events", "warned", "id", id)
		evts[id]["now"], e = sqlGetWithInt("events", "now", "id", id)
	}
	return evts, e
}

// allows padding without annoying fmt usage
func padRight(str string, pad string, length int) string {
	for {
		str += pad
		if len(str) > length {
			return str[:length]
		}
	}
}
func padLeft(str string, pad string, length int) string {
	for {
		str = pad + str
		if len(str) > length {
			return str[len(str)-length:]
		}
	}
}

// update/create profile
func updateProfile(mbr *discordgo.Member, addxp int) {
	uex, e := sqlExists("users", "mention", mbr.User.Mention())
	if e != nil {
		return
	}
	if !uex {
		sqlInsert("users", "mention", mbr.User.Mention())
	}
	if len(mbr.Nick) == 0 {
		mbr.Nick = mbr.User.Username
	}
	sqlUpdate("users", "name", mbr.Nick, "mention", mbr.User.Mention())
	sqlUpdate("users", "joined", mbr.JoinedAt, "mention", mbr.User.Mention())

	xp, e := sqlGetInt("users", "xp", "mention", mbr.User.Mention())
	if e != nil {
		return
	}
	xp += addxp
	lvl := xp / xpPerLvl
	sqlUpdateInt("users", "xp", xp, "mention", mbr.User.Mention())
	sqlUpdateInt("users", "level", lvl, "mention", mbr.User.Mention())
}

func sqlExists(table string, whereField string, whereValue string) (bool, error) {
	e := dbCheck()
	if e != nil {
		return false, e
	}
	q, e := db.Query("select exists(select 1 from "+table+" where "+whereField+" = $1)", whereValue)
	defer q.Close()
	if e != nil {
		trace(e)
		return false, e
	}
	var ex bool
	if q.Next() {
		e = q.Scan(&ex)
		if e != nil {
			trace(e)
			return false, e
		}
	}
	e = q.Err()
	trace(e)
	return ex, e
}

func sqlExistsInt(table string, whereField string, whereValue int) (bool, error) {
	e := dbCheck()
	if e != nil {
		return false, e
	}
	q, e := db.Query("select exists(select 1 from "+table+" where "+whereField+" = $1)", whereValue)
	defer q.Close()
	if e != nil {
		trace(e)
		return false, e
	}
	var ex bool
	q.Next()
	e = q.Scan(&ex)
	if e != nil {
		trace(e)
		return false, e
	}
	return ex, e
}

func sqlGet(table string, field string, whereField string, whereValue string) (string, error) {
	e := dbCheck()
	if e != nil {
		return "", e
	}
	q, e := db.Query("select "+field+" from "+table+" where "+whereField+" = $1", whereValue)
	defer q.Close()
	if e != nil {
		trace(e)
		return "", e
	}
	var ret string
	if q.Next() {
		e = q.Scan(&ret)
		trace(e)
	}
	e = q.Err()
	trace(e)
	return ret, e
}

func sqlGetWithInt(table string, field string, whereField string, whereValue int) (string, error) {
	e := dbCheck()
	if e != nil {
		return "", e
	}
	q, e := db.Query("select "+field+" from "+table+" where "+whereField+" = $1", whereValue)
	defer q.Close()
	if e != nil {
		trace(e)
		return "", e
	}
	var ret string
	if q.Next() {
		e = q.Scan(&ret)
		trace(e)
	}
	e = q.Err()
	trace(e)
	return ret, e
}

func sqlGetInt(table string, field string, whereField string, whereValue string) (int, error) {
	e := dbCheck()
	if e != nil {
		return 0, e
	}
	q, e := db.Query("select "+field+" from "+table+" where "+whereField+" = $1", whereValue)
	defer q.Close()
	if e != nil {
		trace(e)
		return 0, e
	}
	var ret int
	if q.Next() {
		e = q.Scan(&ret)
		trace(e)
	}
	e = q.Err()
	trace(e)
	return ret, e
}

func sqlGetIntList(table string, field string) ([]int, error) {
	e := dbCheck()
	if e != nil {
		return []int{}, e
	}
	q, e := db.Query("select " + field + " from " + table + ";")
	defer q.Close()
	if e != nil {
		trace(e)
		return []int{}, e
	}
	var ret []int
	for q.Next() {
		var i int
		e = q.Scan(&i)
		if e != nil {
			trace(e)
			return ret, e
		}
		ret = append(ret, i)
	}
	return ret, e
}

func sqlInsert(table string, field string, value string) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("insert into "+table+" ("+field+") values ($1)", value)
	trace(e)
	return e
}

func sqlInsertInt(table string, field string, value int) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("insert into "+table+" ("+field+") values ($1)", value)
	trace(e)
	return e
}

func sqlUpdate(table string, field string, value string, whereField string, whereValue string) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("update "+table+" set "+field+" = $1 where "+whereField+" = $2", value, whereValue)
	trace(e)
	return e
}

func sqlUpdateWithInt(table string, field string, value string, whereField string, whereValue int) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("update "+table+" set "+field+" = $1 where "+whereField+" = $2", value, whereValue)
	trace(e)
	return e
}

func sqlUpdateToday(table string, field string, whereField string, whereValue string) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("update "+table+" set "+field+" = current_date() where "+whereField+" = $1", whereValue)
	trace(e)
	return e
}

func sqlUpdateInt(table string, field string, value int, whereField string, whereValue string) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("update "+table+" set "+field+" = $1 where "+whereField+" = $2", value, whereValue)
	trace(e)
	return e
}

func sqlDelete(table string, whereField string, whereValue string) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("delete from "+table+" where "+whereField+" = $1", whereValue)
	trace(e)
	return e
}

func sqlDeleteWithInt(table string, whereField string, whereValue int) error {
	e := dbCheck()
	if e != nil {
		return e
	}
	_, e = db.Exec("delete from "+table+" where "+whereField+" = $1", whereValue)
	trace(e)
	return e
}

func timesInZones(t time.Time) (str string) {
	now := time.Now().In(timeZone)

	hrs := int(t.Sub(now).Hours())
	mins := int(t.Sub(now).Minutes()) % 60

	est, e := time.LoadLocation("America/New_York")
	trace(e)
	cst, e := time.LoadLocation("America/Chicago")
	trace(e)
	pst, e := time.LoadLocation("America/Los_Angeles")
	trace(e)
	gmt, e := time.LoadLocation("Europe/London")
	trace(e)
	nzst, e := time.LoadLocation("Pacific/Auckland")
	trace(e)
	cest, e := time.LoadLocation("Europe/Berlin")
	trace(e)
	pht, e := time.LoadLocation("Asia/Manila")
	trace(e)

	str = "\n\nTimes in Zones:" +
		"```" +
		"\n" + t.In(est).Format("MST "+strings.Repeat("-", 5-len(t.In(est).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"     " + t.In(cst).Format("MST "+strings.Repeat("-", 5-len(t.In(cst).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"\n" + t.In(pst).Format("MST "+strings.Repeat("-", 5-len(t.In(pst).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"     " + t.In(gmt).Format("MST "+strings.Repeat("-", 5-len(t.In(gmt).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"\n" + t.In(nzst).Format("MST "+strings.Repeat("-", 5-len(t.In(nzst).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"     " + t.In(cest).Format("MST "+strings.Repeat("-", 5-len(t.In(cest).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"\n" + t.In(pht).Format("MST "+strings.Repeat("-", 5-len(t.In(pht).Format("MST")))+" Mon 02 Jan -- 15:04 (03:04 PM)") +
		"```" +
		"  -> in " + strconv.Itoa(hrs) + " hrs " + strconv.Itoa(mins) + " mins"
	return
}

func progressBar(s *discordgo.Session, cID string, mID string, pct int) {
	if pct >= 100 {
		s.ChannelMessageDelete(cID, mID)
	} else {
		length := 10
		stars := pct / length
		dashes := length - stars
		s.ChannelMessageEdit(cID, mID, "["+strings.Repeat("*", stars)+strings.Repeat("-", dashes)+"]")
	}
}

func translateMsg(s *discordgo.Session, m *discordgo.Message, l string) {
	str, e := translateTo(l, m.Content)
	if e == nil {
		s.ChannelMessageSend(m.ChannelID, "*[beep]* *["+l+"]*  "+str)
	}
}

func translateTo(t string, s string) (string, error) {
	resp, e := http.Get("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=" + t + "&dt=t&q=" + url.QueryEscape(s))
	if e != nil {
		return "", e
	}
	b, e := ioutil.ReadAll(resp.Body)
	if e != nil {
		return "", e
	}
	var tr [][][]interface{}
	json.Unmarshal(b, &tr)
	var str string
	for _, st := range tr[0] {
		str += fmt.Sprint(st[0])
	}
	if str == s {
		return "", errors.New("translateTo: nothing to translate")
	}
	return str, e
}

func ytSearch(s string) ([]string, error) {
	apiResponse, e := http.Get("https://www.googleapis.com/youtube/v3/search?part=snippet&q=" + url.QueryEscape(s) + "&maxResults=10&key=AIzaSyAyhDJxj2do9CnvoK3vmUEcgwvdeeVHDIc")
	if e != nil {
		return []string{}, e
	}
	apiData, e := ioutil.ReadAll(apiResponse.Body)
	if e != nil {
		return []string{}, e
	}
	var ytData ytSnippet
	e = json.Unmarshal(apiData, &ytData)
	if e != nil {
		return []string{}, e
	}
	vIDs := []string{}
	for _, item := range ytData.Items {
		if len(item.ID.VideoID) > 0 {
			vIDs = append(vIDs, item.ID.VideoID)
		}
	}
	return vIDs, e
}

func ddgSearch(s string) (ddgResult, error) {
	apiResponse, e := http.Get("http://api.duckduckgo.com/?format=json&q=" + url.QueryEscape(s))
	if e != nil {
		return ddgResult{}, e
	}
	apiData, e := ioutil.ReadAll(apiResponse.Body)
	if e != nil {
		return ddgResult{}, e
	}
	var res ddgResult
	e = json.Unmarshal(apiData, &res)
	return res, e
}

func cleanBeeps(s *discordgo.Session, g *discordgo.Guild) {
	for _, c := range g.Channels {
		ms, e := s.ChannelMessages(c.ID, 100, "", "", "")
		if e != nil {
			continue
		}
		var dms []string
		for _, m := range ms {
			if strings.HasPrefix(m.Content, "*[beep]*") {
				go s.ChannelMessageEdit(c.ID, m.ID, strings.Replace(m.Content, "*[beep]*", "*[boop]*", 1))
			} else if strings.HasPrefix(m.Content, "*[boop]*") {
				dms = append(dms, m.ID)
			}
		}
		if len(dms) > 0 {
			go s.ChannelMessagesBulkDelete(c.ID, dms)
		}
	}
}

func cleanTrash(s *discordgo.Session, g *discordgo.Guild) {
	for _, c := range g.Channels {
		if c.Name == "announcements" {
			continue
		}
		ms, e := s.ChannelMessages(c.ID, 100, "", "", "")
		if e != nil {
			continue
		}
		var dms []string
		for _, m := range ms {
			for _, r := range m.Reactions {
				if r.Emoji.Name == "trash" {
					if r.Count >= trashLimit {
						dms = append(dms, m.ID)
					}
				}
			}
		}
		if len(dms) > 0 {
			go s.ChannelMessagesBulkDelete(c.ID, dms)
		}
	}
}

func remFirst(s []string) []string {
	if len(s) > 1 {
		return s[1:]
	}
	return []string{}
}

func dbCheck() error {
	e := db.Ping()
	if e != nil {
		db.Close()
		db, e = sql.Open(sqlDriver, sqlSource)
		if e != nil {
			db.Close()
			db, e = sql.Open(sqlDriver, sqlSource)
		}
	}
	return e
}

func trace(e error) {
	if e == nil {
		return
	}
	pc := make([]uintptr, 15)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	frame, _ := frames.Next()
	fmt.Printf("%s,:%d %s\n%s\n", frame.File, frame.Line, frame.Function, e.Error())
}
