-- Disposable browser-qualification fixture. Never load into a shared or
-- production database.
INSERT INTO spatial_catalogs(id,status,source_watermark,algorithm_version,published_at,entity_count,link_count,subreddit_count,user_count,post_count,comment_count,min_x,max_x,min_y,max_y,min_z,max_z,validation_result,catalog_checksum)
VALUES(1,'published','2026-08-15T12:00:00Z','catalog-v2-fixture',now(),19,0,8,3,4,4,-800,800,-500,500,-200,200,'{"valid":true}','browser-fixture');

INSERT INTO graph_revisions(id,status,source_watermark,algorithm_version,layout_algorithm,layout_seed,layout_dimensions,published_at,node_count,link_count,community_count,min_x,max_x,min_y,max_y,min_z,max_z,validation_result,spatial_catalog_id,scene_contract)
VALUES(1,'published','2026-08-15T12:00:00Z','affinity-world-v2-fixture','stable-community-3d-v2',74036448965714,3,now(),8,0,16,-800,800,-500,500,-200,200,'{"valid":true}',1,'spatial-scene-v2');
INSERT INTO graph_revision_current(singleton,revision_id) VALUES(true,1);
INSERT INTO spatial_catalog_current(singleton,catalog_id) VALUES(true,1);

INSERT INTO graph_revision_macro_groups(revision_id,macro_group_id,display_name,evidence_label,palette_role,primary_color,member_count,representative_clusters,evidence_metrics) VALUES
(1,'m:makers','Maker Networks','DIY · woodworking · 3Dprinting',0,'#68DCFF',1,'["c:makers"]','{}'),
(1,'m:ecology','Living Systems','ecology · gardening · mycology',1,'#B6FF62',1,'["c:ecology"]','{}'),
(1,'m:stories','Story Worlds','books · fantasy · writing',2,'#FFD166',1,'["c:stories"]','{}'),
(1,'m:sound','Sound Cultures','music · synthesizers · vinyl',3,'#FF7EB6',1,'["c:sound"]','{}'),
(1,'m:civic','Civic Commons','local · transit · urbanism',4,'#A78BFA',1,'["c:civic"]','{}'),
(1,'m:science','Open Inquiry','science · astronomy · data',5,'#4ADE80',1,'["c:science"]','{}'),
(1,'m:games','Play Worlds','indiegames · tabletop · gamedev',6,'#FB923C',1,'["c:games"]','{}'),
(1,'m:care','Care Networks','mentalhealth · mutualaid · disability',7,'#F87171',1,'["c:care"]','{}');

INSERT INTO graph_revision_communities(revision_id,community_id,parent_id,level,label,size,x,y,z,display_name,evidence_label,macro_group_id,primary_color,secondary_color,distinctiveness,affinity_confidence,bridge_affinity,is_bridge,naming_method,naming_version,naming_confidence,representative_members,evidence_metrics) VALUES
(1,'c:makers','m:makers',0,'DIY · woodworking',940,-760,-140,20,'Maker Commons','DIY · woodworking · 3Dprinting','m:makers','#68DCFF',NULL,.91,.94,0,false,'provider','cluster-label-v2',.92,'["DIY","woodworking","3Dprinting"]','{"overview_rank":1}'),
(1,'c:ecology','m:ecology',0,'ecology · gardening',820,-510,330,-90,'Backyard Ecologies','ecology · gardening · mycology','m:ecology','#B6FF62','#68DCFF',.87,.90,.34,true,'provider','cluster-label-v2',.89,'["gardening","mycology"]','{"overview_rank":2}'),
(1,'c:stories','m:stories',0,'books · fantasy',780,-160,-390,110,'Storycraft Circles','books · fantasy · writing','m:stories','#FFD166',NULL,.84,.88,0,false,'provider','cluster-label-v2',.87,'["books","fantasywriters"]','{"overview_rank":3}'),
(1,'c:sound','m:sound',0,'music · synthesizers',730,120,410,-40,'Signal and Sound','music · synthesizers · vinyl','m:sound','#FF7EB6','#A78BFA',.89,.91,.32,true,'provider','cluster-label-v2',.90,'["synthesizers","vinyl"]','{"overview_rank":4}'),
(1,'c:civic','m:civic',0,'transit · urbanism',690,410,-330,70,'Civic Fabric','local · transit · urbanism','m:civic','#A78BFA',NULL,.82,.86,0,false,'provider','cluster-label-v2',.84,'["urbanplanning","transit"]','{"overview_rank":5}'),
(1,'c:science','m:science',0,'science · astronomy',650,690,180,-120,'Open Inquiry','science · astronomy · data','m:science','#4ADE80',NULL,.93,.95,0,false,'provider','cluster-label-v2',.94,'["science","astronomy"]','{"overview_rank":6}'),
(1,'c:games','m:games',0,'indiegames · tabletop',610,330,470,140,'Playful Systems','indiegames · tabletop · gamedev','m:games','#FB923C',NULL,.81,.85,0,false,'provider','cluster-label-v2',.83,'["indiegames","tabletop"]','{"overview_rank":7}'),
(1,'c:care','m:care',0,'mutualaid · disability',590,-300,80,-160,'Care Networks','mentalhealth · mutualaid · disability','m:care','#F87171','#B6FF62',.88,.89,.31,true,'provider','cluster-label-v2',.88,'["mutualaid","disability"]','{"overview_rank":8}');

INSERT INTO graph_revision_communities(revision_id,community_id,parent_id,level,label,size,x,y,z,display_name,evidence_label,macro_group_id,primary_color,naming_method,naming_version,evidence_metrics)
SELECT 1,macro_group_id,NULL,1,evidence_label,member_count,0,0,0,display_name,evidence_label,macro_group_id,primary_color,'deterministic-fallback','macro-v1','{}' FROM graph_revision_macro_groups;

INSERT INTO graph_revision_community_links(revision_id,source_community_id,target_community_id,weight,affinity,observed_evidence,signal_composition) VALUES
(1,'c:ecology','c:makers',720000,.72,182,'{"active_users":0.6,"posting_authors":0.2,"repeat":0.1,"references":0.1}'),
(1,'c:makers','c:science',610000,.61,144,'{"active_users":0.65,"posting_authors":0.2,"repeat":0.1,"references":0.05}'),
(1,'c:sound','c:stories',570000,.57,121,'{"active_users":0.6,"posting_authors":0.25,"repeat":0.1,"references":0.05}'),
(1,'c:care','c:civic',540000,.54,116,'{"active_users":0.55,"posting_authors":0.25,"repeat":0.1,"references":0.1}'),
(1,'c:games','c:stories',490000,.49,98,'{"active_users":0.6,"posting_authors":0.2,"repeat":0.15,"references":0.05}');

INSERT INTO spatial_catalog_entities(catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics)
SELECT 1,'subreddit_'||n,name,'subreddit',1000-n*40,-850+n*190,-260+n*70,-180+n*45,NULL,NULL,NULL,community,'community-seeded',now(),jsonb_build_object('subscribers',100000-n*7000,'activity_count',900-n*50,'unique_users',400-n*20)
FROM (VALUES (1,'woodworking','c:makers'),(2,'gardening','c:ecology'),(3,'fantasywriters','c:stories'),(4,'synthesizers','c:sound'),(5,'urbanplanning','c:civic'),(6,'astronomy','c:science'),(7,'indiegames','c:games'),(8,'mutualaid','c:care')) AS item(n,name,community);

INSERT INTO spatial_catalog_entities(catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics) VALUES
(1,'user_1','orbital_loom','user',420,-700,-100,15,NULL,'subreddit_1',NULL,'c:makers','subreddit-seeded',now(),'{"activity_count":420,"post_count":2,"comment_count":8,"distinct_communities":3}'),
(1,'user_2','moss_signal','user',390,-480,300,-85,NULL,'subreddit_2',NULL,'c:ecology','subreddit-seeded',now(),'{"activity_count":390,"post_count":1,"comment_count":12,"distinct_communities":4}'),
(1,'user_3','civic_echo','user',360,390,-300,65,NULL,'subreddit_5',NULL,'c:civic','subreddit-seeded',now(),'{"activity_count":360,"post_count":1,"comment_count":9,"distinct_communities":2}'),
(1,'post_1','A hand-cut joint library','post',280,-730,-120,18,'subreddit_1','subreddit_1','user_1','c:makers','subreddit-seeded',now(),'{"score":280,"comment_count":2,"created_at":"2026-08-12T10:00:00Z"}'),
(1,'post_2','Restoring a shaded native garden','post',250,-500,315,-82,'subreddit_2','subreddit_2','user_2','c:ecology','subreddit-seeded',now(),'{"score":250,"comment_count":1,"created_at":"2026-08-13T10:00:00Z"}'),
(1,'post_3','Night bus network field notes','post',220,405,-315,72,'subreddit_5','subreddit_5','user_3','c:civic','subreddit-seeded',now(),'{"score":220,"comment_count":1,"created_at":"2026-08-14T10:00:00Z"}'),
(1,'post_4','Sensitive field report','post',190,680,170,-118,'subreddit_6','subreddit_6','user_1','c:science','subreddit-seeded',now(),'{"score":190,"comment_count":0,"created_at":"2026-08-14T12:00:00Z"}'),
(1,'comment_1','The grain direction diagram helps.','comment',95,-725,-116,19,'post_1','post_1','user_2','c:makers','post-seeded',now(),'{"score":95,"reply_count":1}'),
(1,'comment_2','Try a narrower chisel first.','comment',75,-720,-112,20,'comment_1','post_1','user_1','c:makers','post-seeded',now(),'{"score":75,"reply_count":0}'),
(1,'comment_3','Shade-tolerant sedges worked here.','comment',80,-495,320,-80,'post_2','post_2','user_3','c:ecology','post-seeded',now(),'{"score":80,"reply_count":0}'),
(1,'comment_4','The transfer timing is the key constraint.','comment',70,410,-310,74,'post_3','post_3','user_2','c:civic','post-seeded',now(),'{"score":70,"reply_count":0}');

INSERT INTO spatial_catalog_documents(catalog_id,entity_id,entity_type,title,body,description,source_permalink,source_sensitive,administrative_sensitive,content_created_at,content_updated_at,provenance)
SELECT 1,id,type,
 CASE WHEN type='post' AND id<>'post_4' THEN label ELSE NULL END,
 CASE WHEN type='comment' THEN label WHEN id='post_1' THEN 'A browsable reference of joints, tools, and failure modes.' WHEN id='post_2' THEN 'A five-year restoration log with soil and canopy notes.' WHEN id='post_3' THEN 'Observed transfers and service gaps across three night routes.' WHEN id='post_4' THEN 'Details intentionally collapsed in the interface.' ELSE NULL END,
 CASE WHEN type='subreddit' THEN 'A public community description for the browser qualification fixture.' ELSE NULL END,
 'https://www.reddit.com/r/example/comments/'||id,false,false,now(),now(),'{"source":"browser-fixture","public":true}'
FROM spatial_catalog_entities;
UPDATE spatial_catalog_documents SET source_sensitive=true WHERE entity_id='post_4';

SELECT setval('spatial_catalogs_id_seq',1,true);
SELECT setval('graph_revisions_id_seq',1,true);
