use resvg::{tiny_skia, usvg};
use std::ffi::{c_char, CStr};
use std::sync::{Arc, OnceLock};

static DB: OnceLock<Arc<usvg::fontdb::Database>> = OnceLock::new();

#[no_mangle]
pub extern "C" fn rv_init(dir: *const c_char) -> i32 {
    let dir = unsafe { CStr::from_ptr(dir) }.to_string_lossy().to_string();
    let mut db = usvg::fontdb::Database::new();
    db.load_fonts_dir(&dir);
    let n = db.len() as i32;
    let _ = DB.set(Arc::new(db));
    n
}

/// Renders SVG bytes into `out` (w*h*4 RGBA, premultiplied). Returns 0 on success.
#[no_mangle]
pub extern "C" fn rv_render(svg: *const u8, len: usize, w: u32, h: u32, out: *mut u8) -> i32 {
    let data = unsafe { std::slice::from_raw_parts(svg, len) };
    let mut opt = usvg::Options::default();
    if let Some(db) = DB.get() {
        opt.fontdb = db.clone();
    }
    let tree = match usvg::Tree::from_data(data, &opt) {
        Ok(t) => t,
        Err(_) => return 1,
    };
    let buf = unsafe { std::slice::from_raw_parts_mut(out, (w * h * 4) as usize) };
    let mut pm = match tiny_skia::PixmapMut::from_bytes(buf, w, h) {
        Some(p) => p,
        None => return 2,
    };
    pm.fill(tiny_skia::Color::TRANSPARENT);
    resvg::render(&tree, tiny_skia::Transform::identity(), &mut pm);
    0
}
