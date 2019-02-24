local bot = {}
local botnick = 'testbot1'
local bb = require 'bananaboat'
bot.handlers = {
  ['PRIVMSG'] = function(net, nick, user, host, channel, message)
    if channel ~= botnick then return end
    local target = "" -- nick not set
    if message == "weather" then
      bb.worker(function(target)
	print(string.format("IDIOT ->%s<-", target or 'FUCKING NIL'))
        local bb = require 'bananaboat'
        local weather = bb.owm("key", "johannesburg,za")
	print(string.format("WTF ->%s<-", weather))
        return { {command = 'PRIVMSG', params = {target, weather}} }
      end, target)
    end
  end,
}
bot.servers = {
  test = {
    server = 'localhost',
    tls = false,
  },
}
bot.nick = botnick
bot.username = 'a'
bot.realname = 'e'
return bot
