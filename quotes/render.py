from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
root=Path(__file__).resolve().parents[1]
base=Image.open(root/"art/reminder_cloud.png").convert("RGBA")
font=ImageFont.truetype("/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",16,index=3)
seen=set()
for category,start in [("workhorse",100),("nonsense",200)]:
 lines=(root/"quotes"/f"{category}.txt").read_text(encoding="utf-8").splitlines()
 assert len(lines)==100
 for i,line in enumerate(lines,start):
  assert line and line not in seen
  seen.add(line)
  img=base.copy()
  draw=ImageDraw.Draw(img)
  if draw.textlength(line,font=font)<=270:
   paragraphs=[line]
  else:
   splits=[n for n in range(1,len(line)) if line[n] not in "，。？！、」）" and max(draw.textlength(line[:n],font=font),draw.textlength(line[n:],font=font))<=270]
   assert splits,line
   n=min(splits,key=lambda n:abs(draw.textlength(line[:n],font=font)-draw.textlength(line[n:],font=font))-(18 if line[n-1] in "，、" else 0))
   paragraphs=[line[:n],line[n:]]
  for row,paragraph in enumerate(paragraphs):
   y=63+(row-(len(paragraphs)-1)/2)*23
   draw.text((170,y),paragraph,font=font,fill=(67,48,45,255),anchor="mm")
  img.save(root/"art/bubbles"/f"{i:03d}.png",optimize=True)
