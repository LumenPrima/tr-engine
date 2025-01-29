const router = require('express').Router();
const { query, param, validationResult } = require('express-validator');
const mongoose = require('mongoose');
const { errorHandler } = require('../middleware');
const logger = require('../../utils/logger');
const timestamps = require('../../utils/timestamps');

// Get talkgroups with advanced filtering
router.get('/', 
  [
    query('sys_name').optional().isString(),
    query('sys_num').optional().isInt(),
    query('talkgroup').optional().isInt(),
    query('emergency').optional().isBoolean(),
    query('category').optional().isString(),
    query('group').optional().isString()
  ],
  async (req, res) => {
    try {
      const errors = validationResult(req);
      if (!errors.isEmpty()) {
        return res.status(400).json({ 
          status: 'error',
          errors: errors.array(),
          timestamp: timestamps.getCurrentTimeISO()
        });
      }

      const filter = {};
      if (req.query.sys_name) filter.sys_name = req.query.sys_name;
      if (req.query.sys_num) filter.sys_num = parseInt(req.query.sys_num);
      if (req.query.talkgroup) filter.talkgroup = parseInt(req.query.talkgroup);
      if (req.query.emergency) filter.emergency = req.query.emergency === 'true';
      if (req.query.category) filter.category = req.query.category;
      if (req.query.group) filter.group = req.query.group;

      const collection = mongoose.connection.db.collection('talkgroups');
      const talkgroups = await collection.find(filter).toArray();

        const formattedTalkgroups = talkgroups.map(tg => ({
            talkgroup: tg.talkgroup,
            sys_name: tg.sys_name,
            talkgroup_tag: tg.alpha_tag,
            description: tg.description,
            category: tg.category,
            group: tg.group,
            priority: tg.priority || false,
            encrypted: tg.encrypted || false
        }));

      res.json({
        status: 'success',
        count: formattedTalkgroups.length,
        data: formattedTalkgroups,
        timestamp: timestamps.getCurrentTimeISO()
      });
    } catch (err) {
      logger.error('Error getting talkgroups:', err);
      errorHandler(err, req, res);
    }
  }
);

// Get talkgroup by ID
router.get('/:talkgroup_id', 
  param('talkgroup_id').isInt(),
  async (req, res) => {
    try {
      const errors = validationResult(req);
      if (!errors.isEmpty()) {
        return res.status(400).json({
          status: 'error',
          errors: errors.array(),
          timestamp: timestamps.getCurrentTimeISO()
        });
      }

      const talkgroupId = parseInt(req.params.talkgroup_id);
      const collection = mongoose.connection.db.collection('talkgroups');
      const talkgroup = await collection.findOne({ talkgroup: talkgroupId });
      
      if (!talkgroup) {
        return res.status(404).json({
          status: 'error',
          message: 'Talkgroup not found',
          timestamp: timestamps.getCurrentTimeISO()
        });
      }

      const formattedTalkgroup = {
        talkgroup: talkgroup.talkgroup,
        sys_name: talkgroup.sys_name,
        talkgroup_tag: talkgroup.alpha_tag,
        description: talkgroup.description,
        category: talkgroup.category,
        group: talkgroup.group,
        priority: talkgroup.priority || false,
        encrypted: talkgroup.encrypted || false
      };
      
      res.json({
        status: 'success',
        data: formattedTalkgroup,
        timestamp: timestamps.getCurrentTimeISO()
      });
    } catch (err) {
      logger.error('Error getting talkgroup:', err);
      errorHandler(err, req, res);
    }
  }
);

module.exports = router;
