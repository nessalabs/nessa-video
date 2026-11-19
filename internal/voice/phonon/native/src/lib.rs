//! C interface to Phonon for reel. Adapted from ptts-ffi's CPU engine
//! (github.com/gradium-ai/xn-ptts, MIT OR Apache-2.0): load a checkpoint
//! folder, pick a voice, speak text as 24 kHz mono float PCM.

use ptts::checkpoint::read_config;
use ptts::preprocess::Normalize;
use ptts::synth::{DeviceKind, Quant, SpeechOptions, Synth, SynthBuilder};
use std::cell::RefCell;
use std::ffi::{CStr, CString, c_char, c_void};
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::path::Path;

const WEIGHTS: [(&str, Quant); 2] = [("model.q8.gguf", Quant::Q80), ("model.safetensors", Quant::F32)];

pub struct Handle {
    synth: Synth,
    normalize: Normalize,
    voices: Vec<u8>,
    error: Option<CString>,
}

pub type AudioFn = extern "C" fn(*const f32, usize, *mut c_void) -> bool;

thread_local! {
    static OPEN_ERROR: RefCell<Option<CString>> = const { RefCell::new(None) };
}

fn cstring(s: &str) -> CString {
    CString::new(s.replace('\0', " ")).expect("NULs replaced")
}

fn guarded<T>(f: impl FnOnce() -> Result<T, String>) -> Result<T, String> {
    catch_unwind(AssertUnwindSafe(f)).unwrap_or_else(|p| {
        let msg = p
            .downcast_ref::<&str>()
            .map(|s| s.to_string())
            .or_else(|| p.downcast_ref::<String>().cloned())
            .unwrap_or_else(|| "unknown".into());
        Err(format!("internal error: {msg}"))
    })
}

fn open(dir: &Path, lang: &str) -> Result<Handle, String> {
    let normalize = lang.parse::<Normalize>().map_err(|e| e.to_string())?;
    let cfg = read_config(&dir.join("config.json")).map_err(|e| e.to_string())?;
    let (weights, quant) = WEIGHTS
        .iter()
        .map(|&(name, q)| (dir.join(name), q))
        .find(|(p, _)| p.is_file())
        .ok_or(format!("no weights in {}", dir.display()))?;
    let mut builder = SynthBuilder::new(cfg, weights, Normalize::OFF)
        .tokenizer_file(dir.join("tokenizer.json"))
        .device(DeviceKind::Cpu)
        .quant(quant);
    for (name, path) in ptts::loader::checkpoint_voices(dir) {
        builder = builder.add_voice(name, path);
    }
    let synth = builder.build().map_err(|e| e.to_string())?;
    let mut voices = Vec::new();
    for v in synth.voices() {
        voices.extend_from_slice(v.as_bytes());
        voices.push(0);
    }
    voices.push(0);
    Ok(Handle { synth, normalize, voices, error: None })
}

/// Load the checkpoint in `dir`, normalizing text as `lang` (en, fr, …, none).
/// NULL on failure; reel_phonon_error(NULL) then says why.
///
/// # Safety
/// `dir` and `lang` are NUL-terminated UTF-8.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn reel_phonon_new(dir: *const c_char, lang: *const c_char) -> *mut Handle {
    let dir = unsafe { CStr::from_ptr(dir) }.to_string_lossy().to_string();
    let lang = unsafe { CStr::from_ptr(lang) }.to_string_lossy().to_string();
    match guarded(|| open(Path::new(&dir), &lang)) {
        Ok(h) => Box::into_raw(Box::new(h)),
        Err(e) => {
            OPEN_ERROR.with(|l| *l.borrow_mut() = Some(cstring(&e)));
            std::ptr::null_mut()
        }
    }
}

/// The voices, NUL-separated and ending in a second NUL. Borrowed.
///
/// # Safety
/// `h` comes from reel_phonon_new.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn reel_phonon_voices(h: *const Handle) -> *const c_char {
    unsafe { &*h }.voices.as_ptr() as *const c_char
}

/// Speak `text` in `voice` (empty for the default) at `temperature` (negative
/// for the model's own) with `seed`, sending audio to `cb` as it is made.
///
/// # Safety
/// `h` comes from reel_phonon_new; strings are NUL-terminated UTF-8.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn reel_phonon_speak(
    h: *mut Handle,
    text: *const c_char,
    voice: *const c_char,
    temperature: f32,
    seed: u64,
    cb: AudioFn,
    user: *mut c_void,
) -> bool {
    let h = unsafe { &mut *h };
    let text = unsafe { CStr::from_ptr(text) }.to_string_lossy().to_string();
    let voice = unsafe { CStr::from_ptr(voice) }.to_string_lossy().to_string();
    let result = guarded(|| {
        let opts = SpeechOptions {
            voice: (!voice.is_empty()).then_some(voice),
            temperature: (temperature >= 0.0).then_some(temperature),
            seed: Some(seed),
            ..Default::default()
        };
        let text = h.normalize.apply(&text);
        let stream = h.synth.stream_with(&text, &opts).map_err(|e| e.to_string())?;
        for pcm in stream {
            let pcm = pcm.map_err(|e| e.to_string())?;
            if !pcm.is_empty() && !cb(pcm.as_ptr(), pcm.len(), user) {
                break;
            }
        }
        Ok(())
    });
    match result {
        Ok(()) => true,
        Err(e) => {
            h.error = Some(cstring(&e));
            false
        }
    }
}

/// The last error on `h`, or the last reel_phonon_new failure when `h` is NULL.
///
/// # Safety
/// `h` is NULL or comes from reel_phonon_new.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn reel_phonon_error(h: *const Handle) -> *const c_char {
    let err = if h.is_null() {
        OPEN_ERROR.with(|l| l.borrow().as_ref().map(|c| c.as_ptr()))
    } else {
        unsafe { &*h }.error.as_ref().map(|c| c.as_ptr())
    };
    err.unwrap_or(c"".as_ptr())
}

/// # Safety
/// `h` comes from reel_phonon_new and is not used again.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn reel_phonon_free(h: *mut Handle) {
    if !h.is_null() {
        drop(unsafe { Box::from_raw(h) });
    }
}
