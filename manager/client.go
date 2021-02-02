package manager

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/zqhhh/gomoku/internal/httpserver"
)

type ClientManager struct {
	server *httpserver.Server
}

func (m *ClientManager) Init() error {
	server := httpserver.NewServer(fmt.Sprintf(":%d", httpPort))
	m.server = server
	err := server.Start()
	if err != nil {
		return err
	}
	log.Infof("server in port:%d", httpPort)
	return nil
}

func (m *ClientManager) IsOnline(username string) bool {

	user, ok := manager.UserManager.users[username]
	if ok && user.Online() {
		return true
	}
	return false
}

func NewClientManager() *ClientManager {
	return &ClientManager{}
}
