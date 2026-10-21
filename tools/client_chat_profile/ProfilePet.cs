using UI;

namespace HYJS.ChatProfile
{
    // FormationItemSimpleInfo.petInfo is optional: an absent value means the viewed
    // player has no pet equipped, which the server sends as a missing field.
    // UIPet's constructor dereferences its argument, so build the original UIPet
    // only when pet data exists. No pet is invented, and the equipped-pet path is
    // still the original constructor.
    public static class ProfilePet
    {
        public static UIPet Create(ProtoBuf.Message.PetSimpleInfo data)
        {
            return data == null ? null : new UIPet(data);
        }
    }
}
