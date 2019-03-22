# BananaBoatBot

[![Build Status](https://dev.azure.com/spacenerve/bananaboatbot/_apis/build/status/bananaboatbot.bananaboatbot?branchName=master)](https://dev.azure.com/spacenerve/bananaboatbot/_build/latest?definitionId=1&branchName=master)

A simple IRC bot written in [Go](https://golang.org/) scriptable with [Gopher-Lua](https://github.com/yuin/gopher-lua).

## Features

  * Supports use of multiple servers
  * Reloading of Lua in runtime (to reconfigure handlers & servers)
  * Simple design & operation: easy learning curve & full flexibility
  * Ringbuffer for displaying logs in WebUI
  * Built-in utilities: OpenWeatherMap, Luis.ai, HTML title scraping
  * Reasonable test coverage (is that a feature? oh well)

## Usage

```
Usage of ./bananaboatbot:
  -addr string
        Listening address for WebUI (default "localhost:9781")
  -log-commands
        Log commands received from servers
  -lua string
        Path to Lua script
  -max-reconnect int
        Maximum reconnect interval in seconds (default 3600)
  -ring-size int
        Number of entries in log ringbuffer (default 100)
```

## Scripting

The script passed to the bot on startup defines servers to connect to and hooks for [IRC commands](https://modern.ircdocs.horse/).

It can be reloaded by calling the `/reload` endpoint on the web interface.

~~~lua
-- The script must return a table
local bot = {}
-- The bananaboat library provides optional additional features
local bb = require 'bananaboat'

-- servers key holds server configuration
bot.servers = {
  -- this is the 'friendly name' as passed to functions
  freenode = {
    server = 'irc.freenode.net',
    port = 7000,
    tls = true,
    nick = 'DemoBot',
    realname = 'I am a Demo Bot',
  },
}

-- handlers table holds hooks for commands
bot.handlers = {

  --[[
  Create a function hooking 'PING'
  You probably really want this: there is no built-in PING handling currently
  Parameters passed to functions are as follows:
  (1) `net` is the 'friendly name' of the server
  (2) `nick` is the nickname of the message sender if applicable or nil
  (3) `user` is the username of the message sender if applicable or nil
  (4) `host` is the hostname of the message sender if applicable or nil
  (5+) Other parameters are the unpacked parameters of the message, which may vary
  --]]
  PING = function(net, nick, user, host, p1)
    -- Handler return value should be nil or a numeric table of tables
    return {
      -- Each nested table is a raw command to be sent to the server
      -- The `net` key can be used to send a message to a different server
      {command = 'PONG', params = {p1,}},
    }
  end,
  
  PRIVMSG = function(net, nick, user, host, channel, message)
    if channel == (bot.servers[net].nick or bot.nick) then return end -- Ignore PMs
    -- Try find something that looks like a URL
    local url = string.match(message, '(http[s]*://[%a%-%.%d]*[/]*[^%s]*)')
    if not url then return end
    --[[
    Try get title associated with the URL
    This might take a while, so use `worker` to avoid blocking
    First parameter is a function to be run in a new goroutine
    Additional parameters are parameters to pass to that function
    --]]
    bb.worker(function(url, channel)
      -- We need to pass whatever we need to the function as there is no common scope
      local bb = require 'bananaboat'
      local title = bb.get_title(url)
      if not title then return end
      -- Return value is handled the same way as in the parent function
      return {
        {command = 'PRIVMSG', params = {channel, title}}
      }
    end, url, channel)
  end,
}

bot.nick = 'DefaultNick'
bot.username = 'bot'
bot.realname = 'I am a robot'
return bot
~~~
