local bot = require 'trivial1'
local bb = require 'bananaboat'
bot.handlers.PRIVMSG = function(net, nick, user, host, channel, message)
  if channel ~= bot.nick then return end
  local target = "" -- nick not set
  bb.worker(function(message, target)
    local bb = require 'bananaboat'
    local title = bb.get_title(message)
    return { {command = 'PRIVMSG', params = {target, title}} }
  end, message, target)
end
return bot
