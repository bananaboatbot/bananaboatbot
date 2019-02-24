local bot = {}
local botnick = 'testbot1'
local bb = require 'bananaboat'
bot.handlers = {
  ['PRIVMSG'] = function(net, nick, user, host, channel, message)
    if channel ~= botnick then return end
    local target = "" -- nick not set
    bb.worker(function(message, target)
      local bb = require 'bananaboat'
      local title = bb.get_title(message)
      return { {command = 'PRIVMSG', params = {target, title}} }
    end, message, target)
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
