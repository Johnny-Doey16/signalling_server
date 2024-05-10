package webrtc

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func Init() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Replace with your Redis server address
		Password: "",               // Replace with your Redis password, if any
		DB:       0,                // Use the desired Redis database
	})
}

// Store a room in Redis
func StoreRoom(ctx context.Context, room *Room, roomId string) error {
	// Convert the room struct to a byte slice or string
	roomBytes, err := json.Marshal(room)
	if err != nil {
		return err
	}

	// Store the room data in Redis
	return redisClient.Set(ctx, "room:"+roomId, roomBytes, 0).Err()
}

// Retrieve a room from Redis
func GetRoom(ctx context.Context, roomId string) (*Room, error) {
	roomBytes, err := redisClient.Get(ctx, "room:"+roomId).Bytes()
	if err != nil {
		return nil, err
	}

	var room Room
	if err := json.Unmarshal(roomBytes, &room); err != nil {
		return nil, err
	}

	return &room, nil
}

// Store a user in Redis
func StoreUser(ctx context.Context, roomId, streamId string, user *User) error {
	// Convert the user struct to a byte slice or string
	userBytes, err := json.Marshal(user)
	if err != nil {
		return err
	}

	// Store the user data in Redis
	return redisClient.HSet(ctx, "room:"+roomId+":users", streamId, userBytes).Err()
}

// Retrieve a user from Redis
func GetUser(ctx context.Context, roomId, streamId string) (*User, error) {
	userBytes, err := redisClient.HGet(ctx, "room:"+roomId+":users", streamId).Bytes()
	if err != nil {
		return nil, err
	}

	var user User
	if err := json.Unmarshal(userBytes, &user); err != nil {
		return nil, err
	}

	return &user, nil
}
