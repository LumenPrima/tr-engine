const mongoose = require('mongoose');
const logger = require('../../config/logger');
const TalkgroupSchema = require('../../models/Talkgroup');

class TalkgroupManager {
  constructor() {
    this.Talkgroup = mongoose.model('Talkgroup', TalkgroupSchema);
    this.emergencyTalkgroups = new Set(); // In-memory cache for quick lookup
  }

  async initialize() {
    // Load emergency talkgroups at startup
    const emergencyTgs = await this.Talkgroup.find({ emergency: true });
    emergencyTgs.forEach(tg => this.emergencyTalkgroups.add(tg.talkgroup));
  }

  async handleCallActivity(callData) {
    // Update last_heard and create if new (upsert)
    await this.Talkgroup.findOneAndUpdate(
      { sys_num: callData.sys_num, talkgroup: callData.talkgroup },
      {
        $set: {
          sys_name: callData.sys_name,
          alpha_tag: callData.talkgroup_alpha_tag,
          description: callData.talkgroup_description,
          category: callData.talkgroup_tag,
          group: callData.talkgroup_group,
          last_heard: new Date()
        },
        $setOnInsert: {
          first_heard: new Date()
        }
      },
      { upsert: true, new: true }
    );
  }

  async getTalkgroups(filters = {}) {
    const query = {};
    
    if (filters.sys_name) query.sys_name = filters.sys_name;
    if (filters.sys_num) query.sys_num = filters.sys_num;
    if (filters.talkgroup) query.talkgroup = filters.talkgroup;
    if (filters.emergency) query.emergency = true;
    if (filters.category) query.category = filters.category;
    if (filters.group) query.group = filters.group;

    return this.Talkgroup.find(query)
      .sort({ sys_num: 1, talkgroup: 1 })
      .lean();
  }

  async updateTalkgroupConfig(talkgroupID, updates) {
    return this.Talkgroup.findByIdAndUpdate(
      talkgroupID,
      { $set: { config: updates } },
      { new: true }
    );
  }
}

module.exports = TalkgroupManager;
