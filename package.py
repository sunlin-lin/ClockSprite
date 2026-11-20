from pathlib import Path
from zipfile import ZipFile,ZipInfo,ZIP_DEFLATED
import struct,stat

here=Path(__file__).resolve().parent
manual=(here/"README.md").read_text(encoding="utf-8")

def fat():
 binary=bytearray(1<<14)
 struct.pack_into(">II",binary,0,0xcafebabe,2)
 for i,arch in enumerate(("amd64","arm64")):
  data=(here/f"Unified-{arch}").read_bytes()
  magic,cpu,subtype,kind=struct.unpack_from("<IIII",data)
  assert (magic,kind)==(0xfeedfacf,2)
  assert cpu==[0x01000007,0x0100000c][i]
  binary.extend(bytes((-len(binary))%(1<<14)))
  struct.pack_into(">IIIII",binary,8+i*20,cpu,subtype,len(binary),len(data),14)
  binary.extend(data)
 return binary
def put(archive,name,data,mode=0o644):
 info=ZipInfo(name)
 info.create_system=3
 info.external_attr=(stat.S_IFREG|mode)<<16
 archive.writestr(info,data,compress_type=ZIP_DEFLATED)
for filename in ("ClockSprite.zip","ClockSprite-macOS.zip"):
 output=here/filename
 with ZipFile(output,"w",compression=ZIP_DEFLATED) as bundle:
  put(bundle,"README.md",manual)
  if filename=="ClockSprite.zip":
   exe="ClockSprite.exe"
   put(bundle,exe,(here/exe).read_bytes(),0o755)
  else:
   root="ClockSprite.app/Contents/"
   put(bundle,root+"Info.plist",(here/"Info.plist").read_bytes())
   put(bundle,root+"PkgInfo",b"APPL????")
   put(bundle,root+"Resources/ClockSprite.icns",(here/"art/ClockSprite.icns").read_bytes())
   exe=root+"MacOS/ClockSprite"
   put(bundle,exe,fat(),0o755)
 with ZipFile(output) as bundle:
  assert bundle.testzip() is None
  assert bundle.getinfo(exe).external_attr>>16 & stat.S_IXUSR
  data=bundle.read(exe)
  if filename=="ClockSprite.zip":
   assert data[:2]==b"MZ"
  else:
   assert struct.unpack_from(">II",data)==(0xcafebabe,2)
  assert all("Checklist.app" not in name and "Checklist.exe" not in name for name in bundle.namelist())
  print(output.name,output.stat().st_size,"verified")
