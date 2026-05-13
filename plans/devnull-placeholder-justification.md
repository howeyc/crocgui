# Why `os.DevNull` Placeholder is Used in `filesInfo`


## The Core Problem

The [`client.Send(filesInfo, ...)`] method is the **single entry point** into the croc protocol. It performs several critical functions simultaneously:

1. **PAKE key exchange** — establishing a shared secret key using a password (SharedSecret)
2. **Channel encryption** — creating an encrypted tunnel between sender and receiver
3. **File metadata transfer** — information about the files being sent
4. **Actual data transfer** — binary file stream

## Why the Placeholder is Needed

In **WebDAV / TCP forwarding mode**, real files are transferred **outside of croc** — via the WebDAV server TCP forwarding. However, it is still necessary to:

- ✅ Establish a **secure encrypted tunnel** (PAKE + encryption)
- ✅ Complete **password-based authentication** (SharedSecret)
- ✅ Get confirmation that the **receiver has connected** (`client.Step1ChannelSecured`)
- ❌ **NOT transfer** real files through the croc protocol

The `os.DevNull` placeholder solves this elegantly: croc receives a "file" that:

- **Exists** in the filesystem — no errors when opening
- **Has zero size** — transfer completes instantly
- **Contains no data** — nothing extra is sent


## Analogy

It is like sending an **empty envelope** through a courier service so that:

- The courier (croc) establishes a secure route (encrypted channel)
- The recipient confirms their identity (PAKE authentication)
- After that, the real cargo travels via a **separate highway** (WebDAV TCP forwarding or other app)

## Conclusion

The `os.DevNull` placeholder is **not a hack, but an architectural decision** that enables reusing croc's secure connection infrastructure (PAKE + encryption + authentication) as a **transport tunnel** for WebDAV or other app, without sending any real files through the croc protocol itself.
