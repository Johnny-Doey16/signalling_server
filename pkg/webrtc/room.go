package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/gofiber/websocket/v2"
	"github.com/pion/webrtc/v3"
)

// func RoomConn(c *websocket.Conn, p *Peers, u *User) {
func RoomConn(c *websocket.Conn, r *Room) {
	var config webrtc.Configuration
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		config = turnConfig
	}
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}
	defer peerConnection.Close()

	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	newPeer := PeerConnectionState{
		PeerConnection: peerConnection,
		Websocket: &ThreadSafeWriter{
			Conn:  c,
			Mutex: sync.Mutex{},
		},
		// User: *u,
	}

	// Initialize the User map
	if r.User == nil {
		r.User = make(map[string]*User)
	}

	// Add our new PeerConnection to global list
	r.Peers.ListLock.Lock()
	r.Peers.Connections = append(r.Peers.Connections, newPeer)
	r.Peers.ListLock.Unlock()

	log.Println(r.Peers.Connections)

	// Trickle ICE. Emit server candidate to client
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := newPeer.Websocket.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	peerConnection.OnConnectionStateChange(func(pp webrtc.PeerConnectionState) {
		switch pp {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Println("Peer Connection State Failed:", err)
			}
		case webrtc.PeerConnectionStateClosed:
			r.Peers.SignalPeerConnections()
		}
	})

	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// Create a track to fan out our incoming video to all peers
		trackLocal := r.Peers.AddTrack(t)
		if trackLocal == nil {
			return
		}
		defer r.Peers.RemoveTrack(trackLocal)

		buf := make([]byte, 1500)
		for {
			i, _, err := t.Read(buf)
			if err != nil {
				return
			}

			if _, err = trackLocal.Write(buf[:i]); err != nil {
				return
			}
		}
	})

	/*
		peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
			switch connectionState {
			case webrtc.ICEConnectionStateFailed:
				// Handle disconnection or failure here
				log.Println("ICE Connection State: Failed")
				handleDisconnectedUser(r, c, newPeer)
			case webrtc.ICEConnectionStateClosed:
				// Handle connection closed here
				log.Println("ICE Connection State: Closed")
				handleDisconnectedUser(r, c, newPeer)
			}
		})
	*/

	r.Peers.SignalPeerConnections()
	message := &websocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}

		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}

		case "join":
			joinData := struct {
				StreamId            string `json:"stream_id"`
				ParticipantId       string `json:"participant_id"`
				ParticipantUsername string `json:"participant_username"`
				ParticipantAvatar   string `json:"participant_avatar"`
				RoomId              string `json:"room_id"`
			}{}

			if err := json.Unmarshal([]byte(message.Data), &joinData); err != nil {

				log.Println(err)
				return
			}
			// Handle the user details, e.g., store them in the individual client connection
			//  Add User to room

			r.User[joinData.StreamId] = &User{
				Username: joinData.ParticipantUsername,
				Avatar:   joinData.ParticipantAvatar,
				Email:    joinData.ParticipantId,
			}
			// log.Println("Room - ", r)
			// fmt.Printf("Room Info %%+v: %+v\n", r)
			fmt.Printf("Room Info: %+v\n", struct {
				User map[string]*User
			}{
				User: r.User,
			})

			// Send a response back to the client
			data, err := json.Marshal(joinData)
			if err != nil {
				log.Println(err)
				return
			}

			response := websocketMessage{
				Event: "join",
				Data:  string(data),
			}

			if err := newPeer.Websocket.WriteJSON(&response); err != nil {
				log.Println(err)
			}

		case "toggleMute":
			// Handle the event indicating a mute state change from another participant
			var toggleMuteData struct {
				IsMuted  bool   `json:"is_muted"`
				StreamId string `json:"stream_id"`
			}

			if err := json.Unmarshal([]byte(message.Data), &toggleMuteData); err != nil {
				log.Println(err)
				return
			}

			// Handle the mute state change (update UI, etc.)
			handleToggleMuteEvent(toggleMuteData.IsMuted)

			// Broadcast the mute state change to all connected peers
			broadcastToggleMuteEvent(r, c, toggleMuteData.IsMuted, toggleMuteData.StreamId)

		case "toggleVideo":
			// Handle the event indicating a video state change from another participant
			var toggleVideoData struct {
				IsVideoEnabled bool   `json:"is_video_enabled"`
				StreamId       string `json:"stream_id"`
			}

			if err := json.Unmarshal([]byte(message.Data), &toggleVideoData); err != nil {
				log.Println(err)
				return
			}

			// Broadcast the mute state change to all connected peers
			broadcastToggleVideoEvent(r, c, toggleVideoData.IsVideoEnabled, toggleVideoData.StreamId)

		case "hangUp":
			// Handle the event indicating user hangup or left group
			var leaveMeetData struct {
				RoomId   string `json:"room_id"`
				StreamId string `json:"stream_id"`
			}

			if err := json.Unmarshal([]byte(message.Data), &leaveMeetData); err != nil {
				log.Println(err)
				return
			}

			// Delete the user stream id from the room
			RoomsLock.Lock()
			delete(r.User, leaveMeetData.StreamId)
			RoomsLock.Unlock()
		}
	}
}

func handleToggleMuteEvent(isMuted bool) {
	// Update UI or perform actions based on the mute state change
	log.Printf("Received toggleMute event. Mic state: %t\n", isMuted)
}

func broadcastToggleMuteEvent(r *Room, sender *websocket.Conn, isMuted bool, streamId string) {
	message := websocketMessage{
		Event: "toggleMute",
		Data:  fmt.Sprintf(`{"is_muted":%t, "stream_id":"%s"}`, isMuted, streamId),
	}

	r.Peers.ListLock.Lock()
	defer r.Peers.ListLock.Unlock()

	for _, peer := range r.Peers.Connections {
		if peer.Websocket.Conn != sender {
			if err := peer.Websocket.WriteJSON(&message); err != nil {
				log.Println(err)
			}
		}
	}
}

func broadcastToggleVideoEvent(r *Room, sender *websocket.Conn, isVideoEnabled bool, streamId string) {
	message := websocketMessage{
		Event: "toggleVideo",
		Data:  fmt.Sprintf(`{"is_video_enabled":%t, "stream_id":"%s"}`, isVideoEnabled, streamId),
	}

	r.Peers.ListLock.Lock()
	defer r.Peers.ListLock.Unlock()

	for _, peer := range r.Peers.Connections {
		if peer.Websocket.Conn != sender {
			if err := peer.Websocket.WriteJSON(&message); err != nil {
				log.Println(err)
			}
		}
	}
}

/*func handleDisconnectedUser(r *Room, c *websocket.Conn, peer PeerConnectionState) {
	// Find and remove the disconnected user's streams
	RoomsLock.Lock()
	defer RoomsLock.Unlock()

	for streamID, user := range r.User {
		if user.Websocket == peer.Websocket {
			// Remove the user's tracks from the room
			r.Peers.RemoveUserTracks(streamID)
			delete(r.User, streamID)

			// Broadcast the hangUp event to notify other participants
			hangUpData := struct {
				RoomId   string `json:"room_id"`
				StreamId string `json:"stream_id"`
			}{
				RoomId:   r.ID,
				StreamId: streamID,
			}

			response := websocketMessage{
				Event: "hangUp",
				Data:  string(marshalJSON(hangUpData)),
			}

			r.Peers.BroadcastExcept(c, &response)
			log.Printf("User disconnected: %s\n", streamID)
			break
		}
	}
}
*/

// Helper function to marshal JSON
func marshalJSON(data interface{}) []byte {
	result, err := json.Marshal(data)
	if err != nil {
		log.Println("Error marshaling JSON:", err)
	}
	return result
}
