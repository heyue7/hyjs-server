"""Rasterize the actual pet mesh + original UV texture; never substitute another pet."""
import argparse,math,json,hashlib
from pathlib import Path
import numpy as np
from PIL import Image,ImageDraw
ROOT=Path(__file__).resolve().parents[2]
def readobj(path):
 vs=[];uv=[];fs=[]
 for line in path.read_text().splitlines():
  p=line.split()
  if not p:continue
  if p[0]=='v':vs.append(list(map(float,p[1:4])))
  elif p[0]=='vt':uv.append(list(map(float,p[1:3])))
  elif p[0]=='f':
   f=[tuple(int(k)-1 if k else -1 for k in q.split('/')) for q in p[1:]]
   for i in range(1,len(f)-1):fs.append([f[0],f[i],f[i+1]])
 return np.array(vs),np.array(uv),fs

def render(pet,yaw,pitch=12,size=384):
 src=ROOT/'analysis/growth75/pet-source';v,uv,faces=readobj(src/(pet+'.obj'));v=v[:,[0,2,1]];v[:,2]*=-1;tex=np.array(Image.open(src/(pet+'-'+pet+'_n.png')).convert('RGBA'))
 theta,phi=math.radians(yaw),math.radians(pitch);right=np.array([math.cos(theta),0,-math.sin(theta)]);forward=np.array([math.sin(theta)*math.cos(phi),math.sin(phi),math.cos(theta)*math.cos(phi)]);up=np.cross(forward,right)
 proj=np.stack([v@right,v@up,v@forward],axis=1);lo=proj[:,:2].min(0);hi=proj[:,:2].max(0);scale=(size*.86)/max(hi-lo);center=(lo+hi)/2
 proj[:,:2]=(proj[:,:2]-center)*scale+size/2;proj[:,1]=size-proj[:,1]
 pix=np.zeros((size,size,4),dtype=np.uint8);depth=np.full((size,size),-np.inf)
 for face in faces:
  ind=[p[0] for p in face];tri=proj[ind];tuv=uv[[p[1] for p in face]];a,b,c=tri
  x0=max(0,int(np.floor(tri[:,0].min())));x1=min(size-1,int(np.ceil(tri[:,0].max())));y0=max(0,int(np.floor(tri[:,1].min())));y1=min(size-1,int(np.ceil(tri[:,1].max())))
  if x0>x1 or y0>y1:continue
  xx,yy=np.meshgrid(np.arange(x0,x1+1)+.5,np.arange(y0,y1+1)+.5)
  den=(b[1]-c[1])*(a[0]-c[0])+(c[0]-b[0])*(a[1]-c[1])
  if abs(den)<1e-9:continue
  w0=((b[1]-c[1])*(xx-c[0])+(c[0]-b[0])*(yy-c[1]))/den;w1=((c[1]-a[1])*(xx-c[0])+(a[0]-c[0])*(yy-c[1]))/den;w2=1-w0-w1
  zz=w0*a[2]+w1*b[2]+w2*c[2];view=depth[y0:y1+1,x0:x1+1];mask=(w0>=0)&(w1>=0)&(w2>=0)&(zz>view)
  if not mask.any():continue
  u=w0*tuv[0,0]+w1*tuv[1,0]+w2*tuv[2,0];tv=w0*tuv[0,1]+w1*tuv[1,1]+w2*tuv[2,1];tx=np.clip((u*tex.shape[1]).astype(int),0,tex.shape[1]-1);ty=np.clip(((1-tv)*tex.shape[0]).astype(int),0,tex.shape[0]-1);col=tex[ty,tx].copy();normal=np.cross(v[ind[1]]-v[ind[0]],v[ind[2]]-v[ind[0]]);normal/=max(1e-10,np.linalg.norm(normal));light=np.array([.3,.7,.6]);light/=np.linalg.norm(light);shade=.76+.24*abs(normal@light);col[:,:,:3]=np.clip(col[:,:,:3]*shade,0,255).astype(np.uint8);col[:,:,3]=255
  pix[y0:y1+1,x0:x1+1][mask]=col[mask];view[mask]=zz[mask]
 return Image.fromarray(pix)
def main():
 out=ROOT/'analysis/growth75/pet-renders';out.mkdir(exist_ok=True);sheet=Image.new('RGB',(384*4,410*3),(64,64,64));draw=ImageDraw.Draw(sheet)
 for row,pet in enumerate(['pet_030','pet_031','pet_032']):
  for col,yaw in enumerate([-40,0,40,90]):
   image=render(pet,yaw);image.save(out/(pet+'-'+str(yaw)+'.png'));sheet.paste(image,(col*384,row*410),image);draw.text((col*384+10,row*410+386),pet+' yaw '+str(yaw),fill='white')
 sheet.save(out/'contact-sheet.jpg')
if __name__=='__main__':main()
