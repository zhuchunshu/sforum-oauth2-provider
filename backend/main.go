package main

import (
	"log"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
)

// SForum OAuth2 Provider 插件入口：SForum 作为 OAuth2 授权服务器。
func main() {
	server, err := newOAuthServer()
	if err != nil {
		log.Fatalf("configure sforum.oauth2-provider: %v", err)
	}
	pluginv2.Serve(server)
}
