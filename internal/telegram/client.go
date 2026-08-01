package telegram
import (
	"net/http"
	"time"
	"github.com/go-telegram/bot"
)
func NewBot(token, serverURL string, handler bot.HandlerFunc) (*bot.Bot, error) {
	return bot.New(token,
		bot.WithServerURL(serverURL),
		bot.WithWorkers(1),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message"}),
		bot.WithDefaultHandler(handler),
		bot.WithHTTPClient(time.Minute, &http.Client{Timeout: 2 * time.Minute}),
	)
}
