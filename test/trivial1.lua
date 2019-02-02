local bot = {}
local botnick = 'testbot1'
bot.handlers = {
  ['PRIVMSG'] = function(net, nick, user, host, channel, message)
    local ret = {}
    if channel == botnick and message == 'HELLO' then
      table.insert(ret, {command = 'PRIVMSG', params = {nick, 'HELLO'}})
    end
    return ret
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
