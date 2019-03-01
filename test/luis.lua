local bot = require 'trivial1'
local bb = require 'bananaboat'
bot.handlers.PRIVMSG = function(net, nick, user, host, channel, message)
  if channel ~= bot.nick then return end
  bb.worker(function() end)
  bb.worker(function(message, botnick)
    local choices = {
      'hey',
      'hi',
      'howdy',
    }
    local bb = require 'bananaboat'
    local intent, score, ents = bb.luis_predict("westus", "foo", "bar", message)
    if not intent and score and ents then return end
    if intent ~= 'Hello' then return end
    if score ~= 0.5 then return end
    local entity = 'mom'
    for _, ent in ipairs(ents) do
      if ent.type == 'Thing' then
        if ent.score == 0.5 then
          entity = ent.entity
        end
      end
    end
    local greet = choices[bb.random(#choices)]
    return { {command = 'PRIVMSG', params = {botnick, string.format('%s %s', greet, entity)}} }
  end, message, bot.nick)
end
return bot
