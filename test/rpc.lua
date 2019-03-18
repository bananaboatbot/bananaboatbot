local bot = require 'trivial1'
bot.handlers.PRIVMSG = nil
bot.web = {}
local handle_hello = {}
handle_hello.func = function(d)
  return {
    {net = 'test', command = 'PRIVMSG', params = {'foo', d['p1'][1]}}
  }
end
bot.web.hello = handle_hello
local junk = {}
local handle_hello_shared = {use_shared_state = true}
handle_hello_shared.func = function(d)
  junk['hello'] = 'junk'
  return {
    {net = 'test', command = 'PRIVMSG', params = {'foo', d['p1'][1]}}
  }
end
bot.web.hello_shared = handle_hello_shared
bot.web.hello_json = {
  parse_query_string = false,
  parse_json_body = true,
  func = function(_, j)
    return {
      {net = 'test', command = 'PRIVMSG', params = {'foo', j['p1'][1]}}
    }
  end
}
return bot
