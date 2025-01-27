const mongoose = require('mongoose');
const logger = require('../../config/logger');
const Talkgroup = require('../../models/Talkgroup');

class TalkgroupManager {
  constructor() {
    this.Talkgroup = Talkgroup;
    this.emergencyTalkgroups = new Set(); // In-memory cache for quick lookup
  }

  async initialize() {
    try {
      // Load emergency talkgroups at startup
      const emergencyTgs = await this.Talkgroup.find({ emergency: true });
      emergencyTgs.forEach(tg => this.emergencyTalkgroups.add(tg.talkgroup));
      
      logger.info(`Initialized ${this.emergencyTalkgroups.size} emergency talkgroups`);
    } catch (error) {
      logger.error('Failed to initialize emergency talkgroups', error);
    }
  }

  async handleCallActivity(callData) {
    try {
      // Update last_heard and create if new (upsert)
      return await this.Talkgroup.findOneAndUpdate(
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
    } catch (error) {
      logger.error('Error handling call activity', error);
      throw error;
    }
  }

  async getTalkgroups(filters = {}) {
    const query = {};
    
    if (filters.sys_name) query.sys_name = filters.sys_name;
    if (filters.sys_num) query.sys_num = filters.sys_num;
    if (filters.talkgroup) query.talkgroup = filters.talkgroup;
    if (filters.emergency) query.emergency = true;
    if (filters.category) query.category = filters.category;
    if (filters.group) query.group = filters.group;

    try {
      return await this.Talkgroup.find(query)
        .sort({ sys_num: 1, talkgroup: 1 })
        .lean();
    } catch (error) {
      logger.error('Error retrieving talkgroups', error);
      throw error;
    }
  }

  async getTalkgroupById(id) {
    try {
      return await this.Talkgroup.findById(id).lean();
    } catch (error) {
      logger.error(`Error retrieving talkgroup by ID: ${id}`, error);
      throw error;
    }
  }

  async updateTalkgroupConfig(talkgroupID, updates) {
    try {
      return await this.Talkgroup.findByIdAndUpdate(
        talkgroupID,
        { $set: { config: updates } },
        { new: true }
      );
    } catch (error) {
      logger.error(`Error updating talkgroup config: ${talkgroupID}`, error);
      throw error;
    }
  }
}

module.exports = TalkgroupManager;
