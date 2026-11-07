package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func localReply(reply *securityaudit.LocalKeywordReply) *keywordrules.Reply {
	if reply == nil {
		return nil
	}
	return &keywordrules.Reply{Text: reply.Text, Model: reply.Model, Protocol: reply.Protocol, Stream: reply.Stream}
}
func writeLocalKeywordReply(c *gin.Context, decision *securityaudit.Decision) bool {
	if decision == nil || decision.Legacy == nil {
		return false
	}
	return keywordrules.WriteHTTP(c, localReply(decision.Legacy.Reply))
}
func writeLocalKeywordReplyWS(ctx context.Context, conn *coderws.Conn, reply *securityaudit.LocalKeywordReply) {
	if reply != nil {
		keywordrules.WriteWS(ctx, conn, localReply(reply))
	}
}
