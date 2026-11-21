// SPDX-License-Identifier: LicenseRef-Reticulum

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EventKind {
    Welcome,
    Joined,
    Parted,
    Msg,
    Notice,
    Action,
    Error,
    Pong,
    Hello,
    Join,
    Part,
    Close,
    Timeout,
    Unknown(i32),
}

impl EventKind {
    fn from_raw(kind: i32) -> Self {
        match kind {
            1 => Self::Welcome,
            2 => Self::Joined,
            3 => Self::Parted,
            4 => Self::Msg,
            5 => Self::Notice,
            6 => Self::Action,
            7 => Self::Error,
            8 => Self::Pong,
            9 => Self::Hello,
            10 => Self::Join,
            11 => Self::Part,
            12 => Self::Close,
            13 => Self::Timeout,
            other => Self::Unknown(other),
        }
    }
}

#[derive(Debug, Clone)]
pub struct Event {
    pub kind: EventKind,
    pub sender: Vec<u8>,
    pub peer: Vec<u8>,
    pub room: String,
    pub nick: String,
    pub body: String,
    pub msg_type: u64,
    pub room_truncated: bool,
    pub nick_truncated: bool,
    pub body_truncated: bool,
}

impl Event {
    fn from_c(ev: &ffi::RrcEvent) -> Self {
        Self {
            kind: EventKind::from_raw(ev.kind),
            sender: ev.sender[..ev.sender_len].to_vec(),
            peer: ev.peer[..ev.peer_len].to_vec(),
            room: cstr_to_string(&ev.room),
            nick: cstr_to_string(&ev.nick),
            body: cstr_to_string(&ev.body),
            msg_type: ev.msg_type,
            room_truncated: ev.room_truncated != 0,
            nick_truncated: ev.nick_truncated != 0,
            body_truncated: ev.body_truncated != 0,
        }
    }
}

pub fn poll_client(client: u64, timeout_ms: i32) -> Result<Event> {
    poll_event(client, timeout_ms, true)
}

pub fn poll_hub(hub: u64, timeout_ms: i32) -> Result<Event> {
    poll_event(hub, timeout_ms, false)
}

fn poll_event(handle: u64, timeout_ms: i32, client: bool) -> Result<Event> {
    let mut ev = ffi::RrcEvent {
        kind: 0,
        sender: [0; HASH_LEN],
        sender_len: 0,
        peer: [0; HASH_LEN],
        peer_len: 0,
        room: [0; 128],
        room_truncated: 0,
        nick: [0; 64],
        nick_truncated: 0,
        body: [0; 1024],
        body_truncated: 0,
        msg_type: 0,
    };
    let code = unsafe {
        if client {
            ffi::rrc_client_event_poll(handle, timeout_ms, &mut ev)
        } else {
            ffi::rrc_hub_event_poll(handle, timeout_ms, &mut ev)
        }
    };
    if code == Error::Timeout as i32 {
        return Err(Error::Timeout);
    }
    map_code(code)?;
    Ok(Event::from_c(&ev))
}

fn cstr_to_string(buf: &[i8]) -> String {
    let end = buf.iter().position(|&c| c == 0).unwrap_or(buf.len());
    let bytes: Vec<u8> = buf[..end].iter().map(|&c| c as u8).collect();
    String::from_utf8_lossy(&bytes)
        .trim_end_matches('\0')
        .to_string()
}
