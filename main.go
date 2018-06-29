package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/o1egl/govatar"

	_ "github.com/lib/pq"
)

const (
	sqlDriver = "postgres"
	sqlSource = "postgresql://discord@localhost:26257/discord?sslmode=disable"
)

var (
	db *sql.DB
)

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
	// pass session its handlers
	s.AddHandler(joinHandler)
	s.AddHandler(leaveHandler)
	s.AddHandler(msgHandler)
	s.AddHandler(reactionHandler)
	log.Println("Bot listening...")
	// update status
	s.UpdateStatus(0, "| ??help")
	// keep bot alive
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}

func reactionHandler(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	var e error
	defer trace(e)

	// get user
	u, e := s.User(r.UserID)
	if e != nil {
		return
	}

	// bot reaction
	if u.Bot {
		return
	}

	// get channel
	c, e := s.State.Channel(r.ChannelID)
	if e != nil {
		c, e = s.Channel(r.ChannelID)
		if e != nil {
			return
		}
	}

	// get guild
	g, e := s.State.Guild(c.GuildID)
	if e != nil {
		g, e = s.Guild(c.GuildID)
		if e != nil {
			return
		}
	}

	// map role IDs to names
	role := make(map[string]string)
	rs, e := s.GuildRoles(g.ID)
	if e != nil {
		return
	}
	for _, ro := range rs {
		role[ro.Name] = ro.ID
	}

	// get message
	m, e := s.ChannelMessage(c.ID, r.MessageID)
	if e != nil {
		return
	}

	// get message info from db
	bl, e := sqlGet("messages", "blacklist", "id", m.ID)
	if e != nil {
		return
	}

	// evaluate
	if bl == "true" {

	}
}

// handles new members
func joinHandler(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
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
	// map role IDs to names
	role := make(map[string]string)
	rs, e := s.GuildRoles(g.ID)
	if e != nil {
		return
	}
	for _, r := range rs {
		role[r.Name] = r.ID
	}
	// get guild info from db
	nc, e := sqlGet("guilds", "newcomer_channel", "id", g.ID)
	if e != nil {
		return
	}

	// newcomer review
	go s.ChannelMessageSend(nc, "")

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

	// drop if not command
	if !strings.HasPrefix(m.Content, "??") {
		return
	}

	// get channel
	c, e := s.State.Channel(m.ChannelID)
	if e != nil {
		c, e = s.Channel(m.ChannelID)
		if e != nil {
			return
		}
	}

	// get guild
	g, e := s.State.Guild(c.GuildID)
	if e != nil {
		g, e = s.Guild(c.GuildID)
		if e != nil {
			return
		}
	}

	// get guild member
	gm, e := s.GuildMember(g.ID, m.Author.ID)
	if e != nil {
		return
	}

	// map emojis to names
	emoji := make(map[string]string)
	for _, em := range g.Emojis {
		emoji[em.Name] = em.APIName()
	}

	// map role IDs and mentions to names
	role := make(map[string]string)
	roleMention := make(map[string]string)
	gr, e := s.GuildRoles(g.ID)
	if e != nil {
		return
	}
	for _, r := range gr {
		role[strings.ToLower(r.Name)] = r.ID
		roleMention[strings.ToLower(r.Name)] = "<@&" + r.ID + "> "
	}

	// init permissions
	isAdmin := false

	// find permissions
	for _, r := range gm.Roles {
		switch r {
		case role["Admin"]:
			isAdmin = true
		case role["Administrator"]:
			isAdmin = true
		case role["Administrators"]:
			isAdmin = true
		case role["Admins"]:
			isAdmin = true
		}
	}

	// map channel IDs to names
	channel := make(map[string]string)
	cs, e := s.GuildChannels(g.ID)
	if e != nil {
		return
	}
	for _, c := range cs {
		channel[strings.ToLower(c.Name)] = c.ID
	}

	// split command and arguments
	msg := strings.Split(m.Content, " ")

	// common case
	msg[0] = strings.ToLower(msg[0])

	// basic help command
	if msg[0] == "??help" {
		go s.ChannelMessageDelete(c.ID, m.ID)
		s.ChannelMessageSend(c.ID, "*[beep]* Current commands:"+
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

	// roll a die
	if msg[0] == "??roll" {
		go s.ChannelMessageDelete(c.ID, m.ID)
		if len(msg) < 2 {
			s.ChannelMessageSend(c.ID, "*[beep]* Please specify die or dice to roll. e.g. `?roll 2d6 1d4`")
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
		s.ChannelMessageSend(c.ID, "*[beep]* Results: "+res+"= **"+strconv.Itoa(cnt)+"**")
		return
	}

	// flip a coin
	if msg[0] == "??flip" {
		go s.ChannelMessageDelete(c.ID, m.ID)
		x := rand.Intn(2)
		var flip string
		if x == 0 {
			flip = "HEADS"
		} else {
			flip = "TAILS"
		}
		s.ChannelMessageSend(c.ID, "*[beep]* "+m.Author.Mention()+" flips a coin: ***"+flip+"***")
		return
	}

	if msg[0] == "??avatar" {
		go s.ChannelMessageDelete(c.ID, m.ID)
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
		s.ChannelMessageSendComplex(c.ID, &discordgo.MessageSend{
			Content: "Avatar Generated (" + content + "):",
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
	if !isAdmin {
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
