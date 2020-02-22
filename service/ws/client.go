package ws

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/bzyy/gomoku/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

// https://github.com/gorilla/websocket/blob/master/examples/chat/client.go

const (
	writeWait = 100 * time.Second

	pongWait = 60 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
	ID     string
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	Target *Client //对手
	Room   *Room
}

func (c *Client) readPump() {
	defer func() {

		if c.Target != nil {
			//重置对手指向“我”的指针
			c.Target.Target = nil
		}

		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))

		msg := WsReceive{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Println("非法的消息格式", err)
			continue
		}
		if err = msg.verify(); err != nil {
			log.Println(err)
			continue
		}

		switch msg.MType {
		case roomMsg:
			m := RcvRoomMsg{}
			_ = mapstructure.Decode(msg.Content, &m)

			switch m.Action {
			case "create":
				if roomNumber, err := c.hub.CreateRoom(c); err == nil {
					m.RoomNumber = roomNumber
					msg.Status = true
				} else if roomNumber == 0 {
					msg.Msg = err.Error()
				} else {
					msg.Msg = err.Error()
				}
				msg.Content = m
				message, _ = json.Marshal(msg)
				c.send <- message
				continue
			case "join":
				if err = c.hub.JoinRoom(c, m.RoomNumber); err != nil {
					log.Println(err)
					msg.Msg = err.Error()
				} else {
					msg.Status = true
					if c.hub.Rooms[uint(m.RoomNumber)].FirstMove == c {
						m.IsBlack = true
					}
				}
				msg.Content = m
				message, _ = json.Marshal(msg)
				c.send <- message
				if c.Target != nil {
					msg.Msg = "对手加入成功"
					if m.IsBlack {
						m.IsBlack = false
					} else {
						m.IsBlack = true
					}
					msg.Content = m
					message, _ = json.Marshal(msg)
					c.Target.send <- message
				}
				continue
			}
		case chessWalk:
			m := RcvChessMsg{}
			_ = mapstructure.Decode(msg.Content, &m)

			if c.Target == nil {
				msg.Msg = "对手断开连接了"
				msg.Content = m
				message, _ = json.Marshal(msg)
				c.send <- message
				continue
			}
			if c.Room == nil {
				continue
			}
			if success, info := c.Room.GoSet(c, &m); success {
				msg.Status = true
				msg.Msg = info
			} else {
				msg.Msg = info
			}
			msg.Content = m
			message, _ = json.Marshal(msg)
			c.send <- message
			if c.Target != nil && msg.Status {
				c.Target.send <- message
			}
			continue
		case roomList:
			msg.Content = c.hub.GetRooms()
			message, _ = json.Marshal(msg)
			c.send <- message
			continue
		}
		// message, _ = json.Marshal(msg)
		// c.hub.broadcast <- MainMsg{ID: c.ID, Msg: message}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetReadDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				log.Println(err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func ServeWs(hub *Hub, c *gin.Context) {
	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	//TODO 验证生成的ID(名字)是否已存在
	clientID := util.GetRandomName()
	client.ID = clientID

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) InRoom(roomNumber uint) bool {
	if room, ok := c.hub.Rooms[roomNumber]; ok {
		if room.FirstMove == c || room.LastMove == c || room.Master == c {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}
