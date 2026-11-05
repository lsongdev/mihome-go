# mihome-go

```go
package main

import (
  "log"
  "os"

  "github.com/lsongdev/mihome-go/mihome"
)

func main() {
  client := mihome.NewClient(
    os.Getenv("MI_USERNAME"),
    os.Getenv("MI_PASSWORD"),
  )
  // Optional: keep the device id and tokens between runs.
  client.TokenPath = "/path/to/.mi.token"
```

#### Web panel and HTTP API

`cmd` serves the web panel at `/` and the JSON API at `/api/`. It needs Node.js
once, to install the browser dependencies that Go embeds in the binary.

```sh
make deps
go run ./cmd              # data directory: the current directory
go run ./cmd -d /var/lib/mihome
```

The data directory holds everything the service keeps:

| File | Content |
| --- | --- |
| `config.yaml` | Configuration, required. [`config.yaml`](./config.yaml) is a sample |
| `.env` | Optional environment variables, such as the secrets below |
| `mihome.db` | SQLite cache. It can be deleted at any time |
| `mi.token` | Login state: device id and tokens. Deleting it means a new login, which may ask for a security verification |

[`deploy/mihome.service`](./deploy/mihome.service) runs it as a system service.

Set `MI_USERNAME`, `MI_PASSWORD` and optionally `MI_API_TOKEN` in `.env` or the
environment. With `MI_API_TOKEN`, scripts send `Authorization: Bearer <token>` and
browsers use the token as the basic authentication password; a non-loopback
`api.listen` requires it. If the login asks for a security verification, the panel
links to the page that does it.

Device pages generate their controls from the MIoT specification named by the
device's `spec_type`, in Chinese where a translation exists. Writing to locks and
heating appliances, and running any action, asks for confirmation first.

Homes, devices and speakers are cached for `cache.devices_ttl` (1 minute), and
specifications for `cache.spec_ttl` (30 days). Add `?refresh=1` to bypass the cache
of `/api/homes`, `/api/speakers` and `/api/devices/{did}`. Property values, player
status and conversations are always live.

| Route | Description |
| --- | --- |
| `GET /api/homes` | Homes, including shared ones, with their devices |
| `GET /api/devices/{did}` | One device and its home |
| `GET /api/devices/{did}/spec` | The device's MIoT specification, or `null` |
| `GET /api/devices/{did}/props?iid=2.1` | Read MIoT properties |
| `PUT /api/devices/{did}/props` | Write a property: `{"siid","piid","value"}` |
| `POST /api/devices/{did}/actions` | Run an action: `{"siid","aiid","in"}` |
| `GET /api/speakers` | Speakers |
| `GET /api/speakers/{id}` | One speaker |
| `GET /api/speakers/{id}/conversations?limit=50` | Conversation records |
| `GET /api/speakers/{id}/player` | Player status |
| `POST /api/speakers/{id}/player` | `{"action":"play"\|"pause"\|"stop"}` |
| `PUT /api/speakers/{id}/volume` | `{"volume":0-100}` |
| `POST /api/speakers/{id}/tts` | `{"text":"..."}` |

The API never returns the local control tokens of devices, nor their MAC or IP
addresses. Errors from the Xiaomi cloud are answered with status 502.

*[[repo]/mihome/miio.go](./mihome/miio.go)*

#### MiioRequest

+ HomeRequest
+ [HomeList](#homelist)
+ HomeDevices
+ [HomeDeviceList](#homedevicelist)
+ HomeGetProps
+ HomeSetProps

#### HomeList

Lists every home, including the ones shared with the account, and their devices.
`ListMinaDevices` only returns speakers, and `HomeDeviceList` only returns the
devices owned by the account.

```go
homes, err := client.HomeList()
if err != nil {
  log.Fatal(err)
}
for _, home := range homes {
  devices, err := client.HomeDevices(home)
  if err != nil {
    log.Fatal(err)
  }
  log.Println(home.Name, home.Shared, len(devices))
}
```

#### HomeDeviceList

```go
devices, err := client.HomeDeviceList(&mihome.DeviceListFilter{
  ShowVirtualModel: true,
  ShowHuamiDevices: 1,
})
if err != nil {
  log.Fatal(err)
}
for _, device := range devices {
  log.Println(device.Name, device.Model, device.Did, device.Token)
}
```

*[[repo]/mihome/miot.go](./mihome/miot.go)*

+ MiotGetProps
+ MiotSetProps
+ [MiotAction](#miotaction)

#### MiotAction

<https://miot-spec.org/miot-spec-v2/instances?status=all>

```go
func (c *Client) Speak(did, text string) (H, error) {
  return c.MiotAction(did, []int{5, 1}, []any{text})
}

func (c *Client) WakeUp(did string) (H, error) {
  return c.MiotAction(did, []int{5, 2}, []any{})
}

func (c *Client) PlayRadio(did string) (H, error) {
  return c.MiotAction(did, []int{5, 3}, []any{})
}

func (c *Client) PlayMusic(did string) (H, error) {
  return c.MiotAction(did, []int{5, 4}, []any{})
}
```

*[[repo]/mihome/mina.go](./mihome/mina.go)*

+ MinaRequest
+ [ListMinaDevices](#listminadevices)
+ [RemoteUbusCall](#remoteubuscall)

#### ListMinaDevices

```go
devices, _ := client.ListMinaDevices(1)
for _, device := range devices {
  log.Println(device.Name, device.DeviceID)
}
```

#### RemoteUbusCall

```go
func (c *Client) TextToSpeech(deviceId, text string) (out H, err error) {
  out = make(map[string]any)
  err = c.RemoteUbusCall(deviceId, "mibrain.text_to_speech", H{"text": text}, out)
  return
}
```

```go
did := "f1801b9f-0034-4153-bbf6-6b6453668c26"
client.PlayerSetVolume(did, 30)
resp, err := client.TextToSpeech(did, "你好，我是小爱")
if err != nil {
  log.Fatal(err)
}
log.Println(resp)
```
