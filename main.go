package main

import (
    "runtime"
    "context"
    "io"
    "os"
    "os/signal"
    "strconv"
    "sync"
    "syscall"
    "time"

    "github.com/hajimehoshi/oto"
)

type dualShock4 struct {
    r *os.File
    g *os.File
    b *os.File
    m sync.Mutex
}

const ds4Col = 0x002233

const (
    colorRed = iota
    colorGreen
    colorBlue
)

func main() {
    if len(os.Args) < 2 {
        panic("ll /sys/class/leds/")
    }
    ds4, err := openDualShock4(os.Args[1])
    if err != nil {
        panic(err)
    }
    defer ds4.Close()
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    ctx, cancel := context.WithCancel(context.Background())
    go ds4.visualize(ctx)
    <-quit
    cancel()
    ds4.writeColor(ds4Col)
}

func (d *dualShock4) visualize(ctx context.Context) {
    otoCtx, err := oto.NewContext(44100, 2, 2, 4096)
    if err != nil {
        panic(err)
    }
    player := otoCtx.NewPlayer()
    defer func() {
        player.Close()
        otoCtx.Close()
    }()
    pool := sync.Pool{
        New: func() interface{} { return make([]byte, 4096) },
    }
    datach := make(chan []byte, 8)
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            default:
                buf := pool.Get().([]byte)
                _, err := os.Stdin.Read(buf)
                if err != nil {
                    panic(err)
                }
                datach <- buf
            }
        }
    }()
    colorCh := generateRainbow(ctx)
    var color int
    for {
        select {
        case <-ctx.Done():
            return
        case color = <-colorCh:
            // noop
        case buf := <-datach:
            runtime.LockOSThread()
            player.Write(buf)
            runtime.UnlockOSThread()
            for i := 0; i < len(buf)-1; i++ {
                s1, s2 := buf[i], buf[i+1]
                sample := lerp(int(s1), int(s2), 255)
                interpolated := colorLerp(rgb(sample, sample, sample), color, 200)
                d.writeColor(interpolated)
            }
            pool.Put(buf)
        }
    }
}

func (d *dualShock4) writeColor(color int) error {
    d.m.Lock()
    defer d.m.Unlock()
    r := strconv.Itoa(0xff & color)
    g := strconv.Itoa(0xff & (color >> 8))
    b := strconv.Itoa(0xff & (color >> 16))
    _, err := io.WriteString(d.r, r)
    if err != nil {
        return err
    }
    _, err = io.WriteString(d.g, g)
    if err != nil {
        return err
    }
    _, err = io.WriteString(d.b, b)
    if err != nil {
        return err
    }
    return nil
}

func openDualShock4(id string) (*dualShock4, error) {
    var (
        d   dualShock4
        err error
    )
    d.r, err = os.OpenFile(getPath(id, colorRed), os.O_WRONLY, 0644)
    if err != nil {
        return nil, err
    }
    d.g, err = os.OpenFile(getPath(id, colorGreen), os.O_WRONLY, 0644)
    if err != nil {
        d.Close()
        return nil, err
    }
    d.b, err = os.OpenFile(getPath(id, colorBlue), os.O_WRONLY, 0644)
    if err != nil {
        d.Close()
        return nil, err
    }
    return &d, nil
}

func (d *dualShock4) Close() error {
    if d.r != nil {
        d.r.Close()
    }
    if d.g != nil {
        d.g.Close()
    }
    if d.b != nil {
        d.b.Close()
    }
    return nil
}

func getPath(id string, c int) string {
    device := "/sys/class/leds/" + id
    switch c {
    default:
        return ""
    case colorRed:
        return device + ":red/brightness"
    case colorGreen:
        return device + ":green/brightness"
    case colorBlue:
        return device + ":blue/brightness"
    }
}

func lerp(v0, v1, t int) int {
    return (v0*(255-t) + v1*t) / 255
}

func rgb(r, g, b int) int {
    return r | (g << 8) | (b << 16)
}

func colorLerp(c1, c2, t int) int {
    const thres = 8

    r1 := 0xff & c1
    g1 := 0xff & (c1 >> 8)
    b1 := 0xff & (c1 >> 16)
    if r1 < thres {
        r1 = thres
    }
    if g1 < thres {
        g1 = thres
    }
    if b1 < thres {
        b1 = thres
    }

    r2 := 0xff & c2
    g2 := 0xff & (c2 >> 8)
    b2 := 0xff & (c2 >> 26)
    if r2 < thres {
        r2 = thres
    }
    if g2 < thres {
        g2 = thres
    }
    if b2 < thres {
        b2 = thres
    }

    return rgb(
        lerp(r1, r2, t),
        lerp(g1, g2, t),
        lerp(b1, b2, t),
    )
}

func generateRainbow(ctx context.Context) chan int {
    ch := make(chan int)
    go func() {
        // https://i.pinimg.com/originals/d8/81/b2/d881b2850db572a124dc5c17549c40d6.png
        rainbowColors := []int{
            0xd30094,
            0x82004b,
            0xff0000,
            0x00ff00,
            0x00ffff,
            0x007fff,
            0x0000ff,
        }
        c1, c2 := 0, 1
        i := 0
        interpolated := rainbowColors[0]
        t := time.NewTicker(10 * time.Millisecond)
        defer t.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-t.C:
                success := false
                for !success {
                    first := colorLerp(
                        rainbowColors[c1],
                        rainbowColors[c2],
                        i,
                    )
                    interpolated = colorLerp(interpolated, first, i)
                    select {
                    case <-ctx.Done():
                        return
                    case <-t.C:
                        // retry
                    case ch <- interpolated:
                        success = true
                    }
                    i++
                    if i == 256 {
                        c1 = (c1 + 1) % len(rainbowColors)
                        c2 = (c2 + 1) % len(rainbowColors)
                        i = 0
                    }
                }
            }
        }
    }()
    return ch
}
