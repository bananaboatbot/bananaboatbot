local bot = {}
local botnick = 'testbot1'
bot.handlers = {
  ['PRIVMSG'] = function(net, nick, user, host, channel, message)
    if channel == botnick and message == 'HELLO' then
      return { {command = 'PRIVMSG', params = {nick, 'HELLO'}, log = 'HI'} }
    end
  end,
}
bot.servers = {
  test = {
    server = 'localhost',
  },
}
bot.defaults = {
  nick = botnick,
  username = 'a',
  realname = 'e',
}
return bot
