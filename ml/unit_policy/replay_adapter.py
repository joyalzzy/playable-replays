"""Adapt Edit 3/4 labeled replay movements to the live policy feature schema."""
from __future__ import annotations
import hashlib,json,math
from dataclasses import dataclass,field
from pathlib import Path
from typing import Any,Mapping
from .features import FEATURE_NAMES
from .schema import UNIT_CLASSES,UNIT_POLICIES,strict_json_loads
LABELED_MOVEMENT_SCHEMA='pro-labeled-movements-v1'; MAP_SIZE=15000.; MAP_DIAGONAL=math.sqrt(2.)
CLASS_PROFILES={'tank':(7.,10.),'fighter':(10.,14.),'marksman':(11.,28.),'mage':(9.,24.),'support':(8.,20.),'assassin':(13.,12.)}
POLICY_FOR_CLASS={'tank':'protector','fighter':'skirmisher','marksman':'aggressive','mage':'aggressive','support':'support','assassin':'aggressive'}
RAW_ATTACK={'tank':350.,'fighter':500.,'marksman':850.,'mage':950.,'support':650.,'assassin':425.}
class ReplayFilterError(ValueError):pass
@dataclass(frozen=True,slots=True)
class ReplayPolicyExample:
    features:tuple[float,...]; action:str; move_dx:float|None; move_dy:float|None; weight:float; group_id:str; source:str
@dataclass(frozen=True,slots=True)
class ReplayAdapterConfig:
    min_label_confidence:float=.6;min_feature_coverage:float=.7;min_profile_confidence:float=.6;positive_weight:float=1.;neutral_weight:float=.45;negative_weight:float=.9;map_size:float=MAP_SIZE;include_neutral:bool=True;max_examples:int=250000
    def validate(self):
        for n,v in [('min_label_confidence',self.min_label_confidence),('min_feature_coverage',self.min_feature_coverage),('min_profile_confidence',self.min_profile_confidence)]:
            if not math.isfinite(v) or not 0<=v<=1:raise ValueError(f'{n} must be in [0,1]')
        for n,v in [('positive_weight',self.positive_weight),('neutral_weight',self.neutral_weight),('negative_weight',self.negative_weight),('map_size',self.map_size)]:
            if not math.isfinite(v) or v<=0:raise ValueError(f'{n} must be positive')
        if self.max_examples<1:raise ValueError('max_examples must be at least 1')
@dataclass(slots=True)
class ReplayConversionStats:
    total_rows:int=0;accepted_rows:int=0;filtered_rows:int=0;total_weight:float=0.;groups:set[str]=field(default_factory=set);by_label:dict[str,int]=field(default_factory=dict);by_action:dict[str,int]=field(default_factory=dict);skipped_reasons:dict[str,int]=field(default_factory=dict)
    def skip(self,r):self.filtered_rows+=1;self.skipped_reasons[r]=self.skipped_reasons.get(r,0)+1
    def accept(self,e,label):
        self.accepted_rows+=1;self.total_weight+=e.weight;self.groups.add(e.group_id);self.by_label[label]=self.by_label.get(label,0)+1;self.by_action[e.action]=self.by_action.get(e.action,0)+1
    def to_dict(self):return {'totalRows':self.total_rows,'acceptedRows':self.accepted_rows,'emittedExamples':self.accepted_rows,'filteredRows':self.filtered_rows,'totalWeight':round(self.total_weight,6),'matchGroups':len(self.groups),'byLabel':dict(sorted(self.by_label.items())),'byAction':dict(sorted(self.by_action.items())),'skippedReasons':dict(sorted(self.skipped_reasons.items())),'leakageAudit':{'inputsUseOnlyPreDecisionFields':True,'futureOutcomeFieldsUsedAsFeatures':False,'futureOutcomesUsedOnlyForTargetsAndWeights':True}}
def num(v,d=None):
    if isinstance(v,bool) or not isinstance(v,(int,float)) or not math.isfinite(float(v)):return d
    return float(v)
def clamp(v,lo=0.,hi=1.):return min(hi,max(lo,v))
def pt(v):
    if not isinstance(v,Mapping):return None
    x,y=num(v.get('x')),num(v.get('z',v.get('y')));return None if x is None or y is None else (x,y)
def direction(a,b):
    if b is None:return None
    dx,dy=b[0]-a[0],b[1]-a[1];m=math.hypot(dx,dy);return None if m<=1e-9 else (dx/m,dy/m)
def relative(a,b,size):
    if b is None:return 0.,0.,1.
    dx,dy=(b[0]-a[0])/size,(b[1]-a[1])/size;return dx,dy,clamp(math.hypot(dx,dy)/MAP_DIAGONAL)
def group_id(row):
    s=row.get('source')
    if isinstance(s,Mapping) and isinstance(s.get('packetShard'),str) and isinstance(s.get('matchIndex'),int):raw=f"{s['packetShard']}\0{s['matchIndex']}"
    else:
        sid=row.get('sceneId')
        if not isinstance(sid,str) or not sid:raise ValueError('missing scene/match identity')
        raw=sid
    return 'replay-match:'+hashlib.sha256(raw.encode()).hexdigest()[:20]
def role_info(row):
    rp=row.get('rolePositioning')
    if not isinstance(rp,Mapping):raise ValueError('missing rolePositioning')
    p=rp.get('profile');pc=num(rp.get('profileConfidence'));fc=num(rp.get('featureCoverage'))
    if p not in UNIT_CLASSES or pc is None or fc is None:raise ValueError('invalid rolePositioning')
    return p,pc,fc,str(rp.get('profileSource') or 'unknown')
def safer(row,origin):
    cf=row.get('counterfactual')
    if isinstance(cf,Mapping):
        best=cf.get('bestAlternative')
        if isinstance(best,Mapping) and (d:=direction(origin,pt(best.get('to')))) is not None:return d
    pre=row.get('preFightContext')
    if isinstance(pre,Mapping):
        if (d:=direction(origin,pt(pre.get('allyCentroid')))) is not None:return d
        if (d:=direction(origin,pt(pre.get('enemyCentroid')))) is not None:return -d[0],-d[1]
    d=direction(origin,pt(row.get('to')));return None if d is None else (-d[0],-d[1])
def target(row,profile):
    label=row.get('label');a,b=pt(row.get('from')),pt(row.get('to'))
    if a is None or b is None or (d:=direction(a,b)) is None:raise ValueError('invalid movement geometry')
    distance_raw=num(row.get('movementDistance'),math.hypot(b[0]-a[0],b[1]-a[1])) or 0.;step=clamp(distance_raw/650.,.25,1.)
    rp=row.get('rolePositioning');pre_role=rp.get('pre',{}) if isinstance(rp,Mapping) else {};intent=rp.get('movementIntent',{}) if isinstance(rp,Mapping) else {}
    threat=num(pre_role.get('threatExposure'),0.) or 0.;isolation=num(pre_role.get('isolationRisk'),0.) or 0.;toward=num(intent.get('towardEnemyCentroid'),0.) or 0.
    pre=row.get('preFightContext');state=pre.get('playerState',{}) if isinstance(pre,Mapping) else {};hp=num(state.get('hpRatio'),.5) or .5
    if label=='positive':return 'move',d[0]*step,d[1]*step,'observed-positive'
    if label=='neutral':
        delta=rp.get('delta') if isinstance(rp,Mapping) else None;fit=num(delta.get('profileFitScore')) if isinstance(delta,Mapping) else None
        if (fit is not None and fit>.02) or distance_raw>180:return 'move',d[0]*step,d[1]*step,'observed-neutral-move'
        return 'hold',None,None,'observed-neutral-hold'
    if label!='negative':raise ValueError('unsupported label')
    severe=threat>=.5 or isolation>=.5 or hp<=.55 or toward>=.2 or (profile in {'marksman','mage','support'} and threat>=.38)
    return ('retreat',None,None,'negative-retreat') if severe and safer(row,a) is not None else ('hold',None,None,'negative-hold')
def feature_vector(row,profile,cfg):
    origin=pt(row.get('from'));pre=row.get('preFightContext')
    if origin is None or not isinstance(pre,Mapping):raise ValueError('missing pre-decision context')
    state=pre.get('playerState',{});state=state if isinstance(state,Mapping) else {};hp=clamp(num(state.get('hpRatio'),.5) or .5)
    rp=row.get('rolePositioning');ti=rp.get('trainingInput',{}) if isinstance(rp,Mapping) else {};intent=ti.get('movementIntent',{}) if isinstance(ti,Mapping) else {}
    move_range,attack_range=CLASS_PROFILES[profile];ne=num(pre.get('nearestEnemyDistance'));na=num(pre.get('nearestAllyDistance'));ec=pt(pre.get('enemyCentroid'));ac=pt(pre.get('allyCentroid'))
    edx,edy,ed=relative(origin,ec,cfg.map_size);adx,ady,ad=relative(origin,ac,cfg.map_size)
    if ne is not None:ed=clamp(ne/(cfg.map_size*MAP_DIAGONAL))
    if na is not None:ad=clamp(na/(cfg.map_size*MAP_DIAGONAL))
    xn,yn=clamp(origin[0]/cfg.map_size),clamp(origin[1]/cfg.map_size);edge=1-clamp(min(xn,1-xn,yn,1-yn)/.25)
    allies=max(0.,num(pre.get('nearbyAllies'),0.) or 0.);enemies=max(0.,num(pre.get('nearbyEnemies'),0.) or 0.);adv=clamp((num(pre.get('localNumberAdvantage'),0.) or 0.)/5.,-1.,1.)
    vals={n:0. for n in FEATURE_NAMES};vals.update({'bias':1.,'team_red':float(str(row.get('team','blue')).lower()=='red'),'position_x':xn,'position_y':yn,'hp_ratio':hp,'missing_hp_ratio':1-hp,'move_range':clamp(move_range/20),'attack_range':clamp(attack_range/35),'cooldown_ready':.5,'nearest_enemy_dx':clamp(edx,-1,1),'nearest_enemy_dy':clamp(edy,-1,1),'nearest_enemy_distance':ed,'enemy_in_attack_range':float(ne is not None and ne<=RAW_ATTACK[profile]),'enemy_in_move_attack_range':float(ne is not None and ne<=RAW_ATTACK[profile]+650),'nearest_enemy_hp_ratio':.5,'nearest_enemy_attack_ready':.5,'nearest_enemy_threatens_unit':float(ne is not None and ne<=RAW_ATTACK[profile]+650),'nearest_ally_dx':clamp(adx,-1,1),'nearest_ally_dy':clamp(ady,-1,1),'nearest_ally_distance':ad,'nearest_ally_hp_ratio':.5,'controlled_distance':1.,'local_allies':clamp(allies/5),'local_enemies':clamp(enemies/5),'local_number_advantage':adv,'objective_distance':1.,'edge_pressure':edge,'turn_fraction':clamp(num(pre.get('sceneProgress'),0.) or 0.)})
    for n in UNIT_CLASSES:vals[f'class_{n}']=float(profile==n)
    policy=POLICY_FOR_CLASS[profile]
    for n in UNIT_POLICIES:vals[f'policy_{n}']=float(policy==n)
    if ec is None and (te:=num(intent.get('towardEnemyCentroid'))) is not None:vals['nearest_enemy_dx']=clamp(te,-1,1)*.1
    result=tuple(vals[n] for n in FEATURE_NAMES)
    if not all(math.isfinite(v) for v in result):raise ValueError('nonfinite feature')
    return result
def sample_weight(label,confidence,coverage,profile_conf,source,association,cfg):
    mult={'positive':cfg.positive_weight,'neutral':cfg.neutral_weight,'negative':cfg.negative_weight}[label];explicit=1. if source=='manifest-tacticalClass' else .85;strength=.7+.3*clamp(abs(association))
    return clamp(mult*confidence*coverage*profile_conf*explicit*strength,.05,2.)
def replay_row_to_example(row,cfg=ReplayAdapterConfig()):
    cfg.validate()
    if row.get('schemaVersion')!=LABELED_MOVEMENT_SCHEMA or row.get('verifiedProfessional') is not True:raise ValueError('unsupported/unverified replay row')
    if row.get('trainingEligible') is not True:raise ReplayFilterError('not-training-eligible')
    label=row.get('label')
    if label not in {'positive','neutral','negative'}:raise ReplayFilterError('unsupported-label')
    if label=='neutral' and not cfg.include_neutral:raise ReplayFilterError('neutral-disabled')
    conf=num(row.get('labelConfidence'))
    if conf is None or conf<cfg.min_label_confidence:raise ReplayFilterError('low-label-confidence')
    profile,pc,fc,ps=role_info(row)
    if fc<cfg.min_feature_coverage:raise ReplayFilterError('low-feature-coverage')
    if pc<cfg.min_profile_confidence:raise ReplayFilterError('low-profile-confidence')
    association=num(row.get('outcomeAssociationScore'))
    if association is None:raise ValueError('missing outcomeAssociationScore')
    action,dx,dy,provenance=target(row,profile)
    return ReplayPolicyExample(feature_vector(row,profile,cfg),action,dx,dy,sample_weight(label,conf,fc,pc,ps,association,cfg),group_id(row),f'professional-replay:{provenance}')
def load_labeled_replay_examples(path,cfg=ReplayAdapterConfig()):
    examples=[];stats=ReplayConversionStats();cfg.validate()
    with Path(path).open(encoding='utf-8') as f:
        for i,line in enumerate(f,1):
            if not line.strip():continue
            stats.total_rows+=1
            if len(examples)>=cfg.max_examples:raise ValueError('replay max_examples exceeded')
            try:
                row=strict_json_loads(line)
                if not isinstance(row,Mapping):raise ValueError('row must be object')
                ex=replay_row_to_example(row,cfg)
            except ReplayFilterError as e:stats.skip(str(e));continue
            except Exception as e:raise ValueError(f'invalid labeled replay row at line {i}: {e}') from e
            examples.append(ex);stats.accept(ex,str(row['label']))
    if not examples:raise ValueError('no eligible replay examples')
    return examples,stats
