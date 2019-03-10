local bot = require 'trivial1'
local botnick = bot.nick
-- Make it say GOODBYE instead of HELLO
bot.handlers.PRIVMSG = function(net, nick, user, host, channel, message)
  if channel ~= botnick then return end
  if message == 'HELLO' then
    return { {'PRIVMSG', botnick, 'GOODBYE'}, {log = 'CAU'} }
  end
end
return bot
