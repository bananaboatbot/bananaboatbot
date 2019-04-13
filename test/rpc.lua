local bot = require 'trivial1'
bot.handlers.PRIVMSG = nil
bot.web = {}
handle_hello = function(d)
  return {
    {net = 'test', command = 'PRIVMSG', params = {'foo', d.query['p1'][1]}}
  }
end
bot.web.hello = handle_hello
local junk = {}
local handle_hello_shared = {use_shared_state = true}
handle_hello_shared.func = function(d)
  junk['hello'] = 'junk'
  return {
    {net = 'test', command = 'PRIVMSG', params = {'foo', d.query['p1'][1]}}
  }
end
bot.web.hello_shared = handle_hello_shared
bot.web.hello_json = {
  parse_query_string = false,
  parse_json_body = true,
  func = function(j)
    return {
      {net = 'test', command = 'PRIVMSG', params = {'foo', j.json['p1'][1]}}
    }
  end
}
return bot
