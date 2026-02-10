package main

import (
	"cmp"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/casbin/govaluate"
)

var (
	Token string
)

func init() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {
	// Create a new Discord session using the provided bot token
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}
	// Register the messageCreate func as a callback for MessageCreate events
	dg.AddHandler(messageCreate)

	// In this example, we only care about recieving message events.
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session
	dg.Close()
}

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID || (m.ChannelID != "" && m.ChannelID != "") { // game-chat or wakebot test
		return
	}
	re := regexp.MustCompile(`(?<RollEntire>(?P<NumberOfDice>\d*)d(?P<DiceFace>\d+)((?P<KeepValue>k|kh|kl)(?P<KeepCount>\d+))?)`)
	rollString := m.Content
	if len(rollString) == 0 || rollString[0] != '!' || !re.Match([]byte(rollString)) {
		return
	}
	var diceResultStrings []string
	for loc := re.FindIndex([]byte(rollString)); loc != nil; loc = re.FindIndex([]byte(rollString)) {
		matches := re.FindStringSubmatch(rollString[loc[0]:loc[1]])
		if matches == nil {
			return
		}
		rollEntireIdx := re.SubexpIndex("RollEntire")
		diceNumIdx := re.SubexpIndex("NumberOfDice")
		diceFaceIdx := re.SubexpIndex("DiceFace")
		keepValueIdx := re.SubexpIndex("KeepValue")
		keepCountIdx := re.SubexpIndex("KeepCount")

		var numberOfDiceStr string
		if len(matches[diceNumIdx]) > 0 {
			numberOfDiceStr = matches[diceNumIdx]
		} else {
			numberOfDiceStr = "1"
		}
		numberOfDice, err := strconv.Atoi(numberOfDiceStr)
		if err != nil {
			panic("Number of dice unparseable")
		}
		diceFace, err := strconv.Atoi(matches[diceFaceIdx])
		if err != nil {
			panic("Dice face unparseable")
		}
		type dicePosition struct {
			result     int
			idx        int
			discounted int
		}
		var diceResults []dicePosition
		for i := range numberOfDice {
			result := randRange(1, diceFace+1)
			diceResults = append(diceResults, dicePosition{
				result: result,
				idx:    i,
				// Boolean, int for sorting
				discounted: 0,
			})
		}
		keepValue := matches[keepValueIdx]
		if len(keepValue) > 0 && len(matches[keepCountIdx]) > 0 {
			keepCount, err := strconv.Atoi(matches[keepCountIdx])
			if err != nil {
				panic("Unparseable keep count.")
			}
			slices.SortFunc(diceResults, func(a, b dicePosition) int {
				return cmp.Compare(a.result, b.result)
			})
			if "kl" == keepValue {
				for i := keepCount; i < len(diceResults); i++ {
					diceResults[i].discounted = 1
				}
			} else {
				for i := len(diceResults) - keepCount - 1; i >= 0; i-- {
					diceResults[i].discounted = 1
				}
			}
			slices.SortFunc(diceResults, func(a, b dicePosition) int {
				if a.discounted != b.discounted {
					return cmp.Compare(a.discounted, b.discounted)
				}
				return cmp.Compare(a.idx, b.idx)
			})
		}
		var diceRolls []string
		var discountedRolls []string
		var sum int
		critSuccess := ""
		critFailure := ""
		for i := range diceResults {
			result := diceResults[i]
			if result.discounted == 0 {
				result := diceResults[i].result
				sum += result
				if diceFace == 20 {
					if result == 1 {
						critFailure = " **CRITICAL FAILURE!**"
					}
					if result == 20 {
						critSuccess = " **CRITICAL SUCCESS!**"
					}
				}
				diceRolls = append(diceRolls, strconv.Itoa(result))
			} else {
				discountedRolls = append(discountedRolls, "~~"+strconv.Itoa(diceResults[i].result)+"~~")
			}
		}
		spacer := ""
		if len(discountedRolls) > 0 {
			spacer = ", "
		}
		diceResultsString := fmt.Sprintf("%s - %s (%s%s%s)%s%s", matches[rollEntireIdx], strconv.Itoa(sum), strings.Join(diceRolls, ", "), spacer, strings.Join(discountedRolls, ", "), critSuccess, critFailure)
		diceResultStrings = append(diceResultStrings, diceResultsString)
		rollString = rollString[0:loc[0]] + strconv.Itoa(sum) + rollString[loc[1]:]
	}
	rollWithoutExclamation := strings.TrimSpace(rollString[1:])
	expression, err := govaluate.NewEvaluableExpression(rollWithoutExclamation)
	if err != nil {
		fmt.Println(err)
		return
	}
	final, err := expression.Evaluate(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	f := final.(float64)
	finalFloat := strconv.FormatFloat(f, 'f', -1, 64)
	var fullExpressionWithResult string
	if finalFloat == rollWithoutExclamation {
		fullExpressionWithResult = ""
	} else {
		fullExpressionWithResult = rollWithoutExclamation + " = " + finalFloat + "\n"
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s\n%sTOTAL: %s", strings.Join(diceResultStrings, "\n"), fullExpressionWithResult, finalFloat))
}
